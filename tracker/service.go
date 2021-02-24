package tracker

import (
	"context"
	"fmt"
	"github-bounty/payments"
	"github.com/lightningnetwork/lnd/lnrpc"
	"strconv"
	"sync"
)
var(
	InactiveError = fmt.Errorf("Issue is not active")
)
const (
	handler_id = "bounty"
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

	LndConnect string
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
}

type IssueService struct {
	store          IssueStore
	ghClient       GithubCommenter
	paymentHandler *payments.PaymentHandler

	sync.Mutex
}

func NewIssueService(store IssueStore, ghClient GithubCommenter, paymentHandler *payments.PaymentHandler) *IssueService {
	srv := &IssueService{store: store, ghClient: ghClient, paymentHandler: paymentHandler}
	srv.paymentHandler.AddHandlerFunc(handler_id, srv.HandleBountyInvoice)

	return srv
}

func (srv *IssueService) AddBountyIssue(ctx context.Context, id int64, link string, owner string, repo string, number int64, lndconnect string) (*BountyIssue, error) {
	bountyIssue := &BountyIssue{
		Id:     id,
		Bounty: 0,
		Url:    link,
		Active: true,
		Owner: owner,
		Repo: repo,
		Number: number,
		LndConnect: lndconnect,
	}
	err := srv.store.Add(ctx, bountyIssue)
	if err != nil {
		return nil,err
	}
	commentId, err := srv.ghClient.AddComment(ctx, bountyIssue)
	if err != nil {
		return nil, err
	}
	bountyIssue.CommentId = commentId
	err = srv.store.Update(ctx, bountyIssue)
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
	//clientconn, err := lnd.ConnectFromLndConnectWithTimeout(ctx, bountyIssue.LndConnect, time.Second*5)
	//if err != nil {
	//	return "", fmt.Errorf("unable to connect to lnd %v", err)
	//}
	//defer clientconn.Close()
	//lndClient := lnrpc.NewLightningClient(clientconn)
	inv, err := srv.paymentHandler.AddHandledPayreq(ctx, handler_id, strconv.Itoa(int(id)),
		&lnrpc.Invoice{
		Memo: fmt.Sprintf("Add bounty on %s", id),
		Value: sats,
		})
	if err != nil {
		return "", err
	}
	return inv.PaymentRequest, nil
}

func (srv *IssueService) HandleBountyInvoice(ctx context.Context, invoice *lnrpc.Invoice, data string) error {
	srv.Lock()
	defer srv.Unlock()
	id, err := strconv.Atoi(data)
	if err != nil {
		return err
	}
	bountyIssue, err := srv.store.Get(ctx, int64(id))
	if err != nil {
		return err
	}
	bountyIssue.Bounty += invoice.Value
	err = srv.store.Update(ctx, bountyIssue)
	if err != nil {
		return err
	}
	err = srv.ghClient.UpdateBountyComment(ctx, bountyIssue)
	if err != nil {
		return err
	}
	return nil
}

