package tracker

import (
	"context"
	"encoding/hex"
	"fmt"
	config "github-bounty"
	"github-bounty/lnd"
	"github.com/lightningnetwork/lnd/lnrpc"
	"github.com/lightningnetwork/lnd/lnrpc/invoicesrpc"
	"google.golang.org/grpc"
	"sync"
	"time"
)
var(
	InactiveError = fmt.Errorf("Issue is not active")
)

type BountyIssue struct {
	Id     int64
	Bounty int64
	Url    string
	Active bool
	Owner string
	Repo string
	Number int64
	CommentId int64
	Pubkey string
	TotalPayments int
	LndConnect string
	// map that matches rhash and whether they are paid
	Payments map[string] bool
}


type GithubCommenter interface {
	AddComment(ctx context.Context, bountyIssue *BountyIssue) (int64, error)
	UpdateBountyComment(ctx context.Context,  bountyIssue *BountyIssue) error
	CloseBountyComment(ctx context.Context,  bountyIssue *BountyIssue) error
}

type IssueStore interface {
	Add(context.Context, *BountyIssue) error
	Update(context.Context, *BountyIssue) error
	Get(context.Context, int64) (*BountyIssue, error)
	Delete(context.Context, int64) error
	ListAll(ctx context.Context) ([]*BountyIssue, error)
}

type IssueService struct {
	cfg *config.Config
	store          IssueStore
	ghClient       GithubCommenter
	lndClient		lnrpc.LightningClient
	sync.Mutex
}

func NewIssueService(cfg *config.Config, store IssueStore, ghClient GithubCommenter, lndClient	lnrpc.LightningClient) *IssueService {
	srv := &IssueService{cfg: cfg, store: store, ghClient: ghClient, lndClient: lndClient}

	return srv
}

func (srv *IssueService) AddBountyIssue(ctx context.Context, id int64, link string, owner string, repo string, number int64, lndconnect string) (*BountyIssue, error) {
	var bountyIssue *BountyIssue
	existingIssue, err := srv.store.Get(ctx, id)
	if err != nil && err != ErrDoesNotExist {
		return nil, err
	}
	if existingIssue != nil {
		existingIssue.Active = true
		bountyIssue = existingIssue
		err = srv.ghClient.UpdateBountyComment(ctx, bountyIssue)
	} else {
		if lndconnect == "" {
			lndconnect = srv.cfg.LndConnect
		}
		bountyIssue = &BountyIssue{
			Id:     id,
			Bounty: 0,
			Url:    link,
			Active: true,
			Owner: owner,
			Repo: repo,
			Number: number,
			LndConnect: lndconnect,
			Payments: make(map[string]bool),
		}

		clientconn, err := lnd.ConnectFromLndConnectWithTimeout(ctx, bountyIssue.LndConnect, time.Second*5)
		if err != nil {
			return nil, fmt.Errorf("unable to connect to remote lnd %v", err)
		}
		defer clientconn.Close()
		remoteLndClient := lnrpc.NewLightningClient(clientconn)
		inv, err := remoteLndClient.AddInvoice(ctx, &lnrpc.Invoice{})
		if err != nil {
			return nil, fmt.Errorf("unable to connect to get invoice from remote lnd %v", err)
		}
		payreq, err := srv.lndClient.DecodePayReq(ctx, &lnrpc.PayReqString{PayReq: inv.PaymentRequest})
		if err != nil {
			return nil, fmt.Errorf("unable to decode invoice %v", err)
		}
		bountyIssue.Pubkey = payreq.Destination

		commentId, err := srv.ghClient.AddComment(ctx, bountyIssue)
		if err != nil {
			return nil, err
		}
		bountyIssue.CommentId = commentId
	}


	err = srv.store.Add(ctx, bountyIssue)
	if err != nil {
		return nil,err
	}
	return bountyIssue,nil
}

func (srv *IssueService) CloseIssue(ctx context.Context, id int64) error {
	srv.Lock()
	defer srv.Unlock()
	bountyIssue, err := srv.store.Get(ctx, id)
	if err == ErrDoesNotExist {
		return nil
	}
	if err != nil {
		return err
	}
	bountyIssue.Active = false
	err = srv.store.Update(ctx, bountyIssue)
	if err != nil {
		return err
	}
	err = srv.ghClient.CloseBountyComment(ctx, bountyIssue)
	if err != nil {
		return err
	}
	return nil
}

func (srv *IssueService) GetBountyInvoice(ctx context.Context, id, sats int64) (string, error) {
	bountyIssue, err := srv.store.Get(ctx, id)
	if err != nil {
		return "", err
	}
	if !bountyIssue.Active {
		return "",InactiveError
	}
	clientconn, err := lnd.ConnectFromLndConnectWithTimeout(ctx, bountyIssue.LndConnect, time.Second*5)
	if err != nil {
		return "", fmt.Errorf("unable to connect to lnd %v", err)
	}
	defer clientconn.Close()
	lndClient := lnrpc.NewLightningClient(clientconn)
	expiry := 600
	invoice := &lnrpc.Invoice{
		Memo: fmt.Sprintf("Add bounty on %s", id),
		Value: sats,
		Expiry: int64(expiry),
	}
	inv, err := lndClient.AddInvoice(ctx, invoice)
	if err != nil {
		return "", err
	}
	bountyIssue.Payments[inv.PaymentRequest] = false
	err = srv.store.Update(ctx, bountyIssue)
	if err != nil {
		return "", err
	}

	go srv.ListenPayment(bountyIssue, inv.RHash, inv.PaymentRequest, invoice.Value)

	return inv.PaymentRequest, nil
}

func (srv *IssueService) ListenPayment(issue *BountyIssue, rHash []byte, payreqString string, sats int64) {
	fmt.Printf("started listening on payment invoice %v on %v \n", payreqString, issue)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second * time.Duration(120))
	defer cancel()
	clientconn, err := lnd.ConnectFromLndConnectWithTimeout(ctx, issue.LndConnect, time.Second*10)
	if err != nil {
		fmt.Printf("unable to connect to lnd %v", err)
		return
	}
	defer clientconn.Close()
	invoicesClient := invoicesrpc.NewInvoicesClient(clientconn)
	invoicesSub, err := invoicesClient.SubscribeSingleInvoice(ctx,&invoicesrpc.SubscribeSingleInvoiceRequest{
		RHash: rHash,
	})
	for {
		select {
			case <-ctx.Done():
				return
		default:
			inv, err := invoicesSub.Recv()
			if err != nil {
				fmt.Printf("unable to receive invoice %v", err)
				return
			}
			if inv.State == lnrpc.Invoice_SETTLED {
				err = srv.SettleInvoice(ctx, issue, payreqString, sats)
				if err != nil {
					fmt.Printf("unable to settle invoice %v", err)
					return
				}
			} else if inv.State == lnrpc.Invoice_CANCELED {
				err = srv.RemovePayment(ctx, issue, payreqString)
				if err != nil {
					fmt.Printf("unable to settle invoice %v", err)
					return
				}
			}
		}
	}
}
func (srv *IssueService) SettleInvoice(ctx context.Context, issue *BountyIssue, payreqString string, sats int64) error {
	srv.Lock()
	defer srv.Unlock()
	fmt.Printf("settled invoice %v on %v \n", payreqString, issue)
	issue.Bounty += sats
	issue.TotalPayments += 1
	issue.Payments[payreqString] = true
	err := srv.store.Update(ctx, issue)
	if err != nil {
		return err
	}
	err = srv.ghClient.UpdateBountyComment(ctx, issue)
	if err != nil {
		return err
	}
	return nil
}
func (srv *IssueService) RemovePayment(ctx context.Context, issue *BountyIssue, payreqString string) error {
	srv.Lock()
	defer srv.Unlock()
	delete(issue.Payments, payreqString)
	fmt.Printf("removed invoice %v on %v \n", payreqString, issue)
	err := srv.store.Update(ctx, issue)
	if err != nil {
		return err
	}
	return nil
}

func (srv *IssueService) RecoverPayments(ctx context.Context) error{
	bountyIssues, err := srv.store.ListAll(ctx)
	if err != nil {
		return err
	}
	for _,bountyIssue := range bountyIssues {
		err = srv.handleBountyIssueRecovery(ctx, bountyIssue)
		if err != nil {
			fmt.Printf("error handling recovery ond %v:  %v", bountyIssue, err)
		}
	}
	return nil
}
func (srv *IssueService) checkPayment(ctx context.Context, lndClient lnrpc.LightningClient, issue *BountyIssue, payreqString string ) error{
	payreq, err := srv.lndClient.DecodePayReq(ctx, &lnrpc.PayReqString{PayReq: payreqString})
	if err != nil {
		return err
	}
	rHashBytes, err := hex.DecodeString(payreq.PaymentHash)
	if err != nil {
		return err
	}
	invoice, err := lndClient.LookupInvoice(ctx, &lnrpc.PaymentHash{RHash: rHashBytes})
	if err != nil {
		return err
	}
	switch invoice.State {
	case lnrpc.Invoice_SETTLED:
		err = srv.SettleInvoice(ctx, issue, payreqString,invoice.Value)
		if err != nil{
			return err
		}
		return nil

	case lnrpc.Invoice_CANCELED:
		err = srv.RemovePayment(ctx, issue, payreqString)
		if err != nil{
			return err
		}
		return nil
	case lnrpc.Invoice_OPEN:
		fallthrough
	case lnrpc.Invoice_ACCEPTED:
		go srv.ListenPayment(issue,rHashBytes, payreqString, invoice.Value)
	}
	return nil
}
func (srv *IssueService) handleBountyIssueRecovery(ctx context.Context, issue *BountyIssue) error {
	cc, err := clientConnFromIssue(ctx, issue)
	if err != nil {
		return err
	}
	defer cc.Close()
	for payreqString,v := range issue.Payments {
		if v {
			continue
		}
		err = srv.checkPayment(ctx, lnrpc.NewLightningClient(cc), issue, payreqString)
		if err != nil {
			fmt.Printf("error checking payment ond %s:  %v \n", payreqString, err)
		}
	}
	return nil
}


func clientConnFromIssue(ctx context.Context, issue *BountyIssue) (*grpc.ClientConn, error) {
	clientconn, err := lnd.ConnectFromLndConnectWithTimeout(ctx, issue.LndConnect, time.Second*10)
	if err != nil {
		return nil,err
	}
	return clientconn, err
}

