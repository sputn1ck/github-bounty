package payments

import (
	"context"
	"encoding/hex"
	"github.com/coreos/bbolt"
	"github.com/lightningnetwork/lnd/lnrpc"
	"google.golang.org/grpc"
	"time"
)

type UnhandledPayment struct {
	RHash     string `json:"RHash"`
	HandlerId string `json:"HandlerId"`
	Data      string `json:"Data"`
}

type UnhandledPaymentsStore interface {
	Add(payment *UnhandledPayment) error
	Get(rHash string) (*UnhandledPayment, error)
	ListAll() ([]*UnhandledPayment, error)
	Remove(rHash string) error
}

type LightningClient interface {
	AddInvoice(ctx context.Context, in *lnrpc.Invoice, opts ...grpc.CallOption) (*lnrpc.AddInvoiceResponse, error)
	SubscribeInvoices(ctx context.Context, in *lnrpc.InvoiceSubscription, opts ...grpc.CallOption) (lnrpc.Lightning_SubscribeInvoicesClient, error)
	LookupInvoice(ctx context.Context, in *lnrpc.PaymentHash, opts ...grpc.CallOption) (*lnrpc.Invoice, error)
}

type PaymentHandler struct {
	lnd   LightningClient
	store UnhandledPaymentsStore

	paymentHandlers map[string]func(ctx context.Context, invoice *lnrpc.Invoice, data string) error
}

func NewPaymentHandler(lnd LightningClient, db *bbolt.DB) (*PaymentHandler, error) {
	paymentDb, err := newUnhandledPaymentsStore(db)
	if err != nil {
		return nil, err
	}
	return  &PaymentHandler{
		lnd:             lnd,
		paymentHandlers: make(map[string]func(ctx context.Context, invoice *lnrpc.Invoice, data string) error),
		store:paymentDb,
	}, nil
}

func (p *PaymentHandler) AddHandlerFunc(handlerId string, handler func(ctx context.Context, invoice *lnrpc.Invoice, data string) error) {
	p.paymentHandlers[handlerId] = handler
}

func (p *PaymentHandler) AddHandledPayreq(ctx context.Context, handlerId string, data string, invoice *lnrpc.Invoice) (*lnrpc.AddInvoiceResponse, error) {
	invoiceRes, err := p.lnd.AddInvoice(ctx, invoice)
	if err != nil {
		return nil, err
	}
	err = p.store.Add(&UnhandledPayment{
		RHash:     hex.EncodeToString(invoiceRes.RHash),
		HandlerId: handlerId,
		Data:      data,
	})
	if err != nil {
		return nil, err
	}
	return invoiceRes, nil
}

func (p *PaymentHandler) StartListening(ctx context.Context) error {
	stream, err := p.lnd.SubscribeInvoices(ctx, &lnrpc.InvoiceSubscription{})
	if err != nil {
		return err
	}
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
			res, err := stream.Recv()
			if err != nil {
				return err
			}
			if res.State == lnrpc.Invoice_SETTLED {
				err := p.handleInvoice(ctx, res)
				if err != nil {
					return err
				}

			}

		}
	}
}
func (p *PaymentHandler) RecoverInvoices(ctx context.Context) error {
	unhandledPayments, err := p.store.ListAll()
	if err != nil {
		return err
	}
	for _, unhandledPayment := range unhandledPayments {
		rHashBytes, err := hex.DecodeString(unhandledPayment.RHash)
		if err != nil {
			return err
		}
		inv, err := p.lnd.LookupInvoice(ctx, &lnrpc.PaymentHash{RHash: rHashBytes})
		if err != nil {
			return err
		}
		if inv.State != lnrpc.Invoice_SETTLED && inv.Expiry <= time.Now().UnixNano() {
			err = p.store.Remove(unhandledPayment.RHash)
			if err != nil {
				return err
			}
			continue
		}
		if inv.State == lnrpc.Invoice_SETTLED {
			err = p.handleInvoice(ctx, inv)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (p *PaymentHandler) handleInvoice(ctx context.Context, invoice *lnrpc.Invoice) error {
	unhandledPayment, err := p.store.Get(hex.EncodeToString(invoice.RHash))
	if err != nil {
		return err
	}
	if val, ok := p.paymentHandlers[unhandledPayment.HandlerId]; ok {
		err = val(ctx, invoice, unhandledPayment.Data)
		if err != nil {
			return err
		}
		err = p.store.Remove(hex.EncodeToString(invoice.RHash))
		if err != nil {
			return err
		}
	}
	return nil
}
