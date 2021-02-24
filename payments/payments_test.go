package payments

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"github.com/lightningnetwork/lnd/lnrpc"
	"github.com/prometheus/common/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"testing"
)

type mockLnd struct {
	invoiceChan chan *lnrpc.Invoice

	invoiceMap map[string]*lnrpc.Invoice

	rHashId int
}

func (m *mockLnd) LookupInvoice(ctx context.Context, in *lnrpc.PaymentHash, opts ...grpc.CallOption) (*lnrpc.Invoice, error) {
	return m.invoiceMap[hex.EncodeToString(in.RHash)], nil
}

func (m *mockLnd) SendInvoice(in *lnrpc.Invoice) {
	in.State = lnrpc.Invoice_SETTLED
	m.invoiceChan <- in
}

func (m *mockLnd) SettleInvoice(in *lnrpc.Invoice) {
	m.invoiceMap[hex.EncodeToString(in.RHash)].State = lnrpc.Invoice_SETTLED
}

func (m *mockLnd) AddInvoice(ctx context.Context, in *lnrpc.Invoice, opts ...grpc.CallOption) (*lnrpc.AddInvoiceResponse, error) {
	m.invoiceMap[hex.EncodeToString(in.RHash)] = in
	return &lnrpc.AddInvoiceResponse{
		RHash:          in.RHash,
		PaymentRequest: "payreq",
		AddIndex:       0,
	}, nil
}

type ChanInvoiceClient struct {
	InvoiceChan chan *lnrpc.Invoice
}

func (c *ChanInvoiceClient) Recv() (*lnrpc.Invoice, error) {
	inv := <-c.InvoiceChan
	return inv, nil
}

func (c *ChanInvoiceClient) Header() (metadata.MD, error) {
	return metadata.MD{}, nil
}

func (c *ChanInvoiceClient) Trailer() metadata.MD {
	return metadata.MD{}
}

func (c *ChanInvoiceClient) CloseSend() error {
	return nil
}

func (c *ChanInvoiceClient) Context() context.Context {
	return context.Background()
}

func (c *ChanInvoiceClient) SendMsg(m interface{}) error {
	panic("implement me")
}

func (c *ChanInvoiceClient) RecvMsg(m interface{}) error {
	panic("implement me")
}

func (m *mockLnd) SubscribeInvoices(ctx context.Context, in *lnrpc.InvoiceSubscription, opts ...grpc.CallOption) (lnrpc.Lightning_SubscribeInvoicesClient, error) {
	return &ChanInvoiceClient{
		InvoiceChan: m.invoiceChan,
	}, nil
}

func Test_Payments(t *testing.T) {
	mockLnd := &mockLnd{
		invoiceChan: make(chan *lnrpc.Invoice),
		invoiceMap:  make(map[string]*lnrpc.Invoice),
		rHashId:     0,
	}
	pService, err := NewPaymentHandler(mockLnd, nil)
	if err != nil {
		t.Error(err)
	}
	tdata := "gude"
	thandler := "foo"
	_, tHashBytes, _ := randomHex(32)
	tinv := &lnrpc.Invoice{Memo: "hi", Value: 100, RHash: tHashBytes}
	_, err = pService.AddHandledPayreq(context.Background(), thandler, tdata, tinv)
	if err != nil {
		t.Error(err)
	}
	endChan := make(chan interface{})
	pService.AddHandlerFunc(thandler, func(ctx context.Context, invoice *lnrpc.Invoice, data string) error {
		if data != tdata {
			t.Errorf("Data expected %s, got %s", tdata, data)
		}
		if tinv.Memo != invoice.Memo {
			t.Errorf("invoice expected %s, got %s", tinv.Memo, invoice.Memo)
		}
		close(endChan)
		return nil
	})
	go func() {
		err := pService.StartListening(context.Background())
		if err != nil {
			log.Error(err)
		}
	}()
	mockLnd.SendInvoice(tinv)
	<-endChan

}


func Test_Recovery(t *testing.T) {
	mockLnd := &mockLnd{
		invoiceChan: make(chan *lnrpc.Invoice),
		invoiceMap:  make(map[string]*lnrpc.Invoice),
		rHashId:     0,
	}
	pService, err := NewPaymentHandler(mockLnd, nil)
	if err != nil {
		t.Error(err)
	}
	tdata := "gude"
	thandler := "foo"
	_, tHashBytes, _ := randomHex(32)
	tinv := &lnrpc.Invoice{Memo: "hi", Value: 100, RHash: tHashBytes}
	_, err = pService.AddHandledPayreq(context.Background(), thandler, tdata, tinv)
	if err != nil {
		t.Error(err)
	}
	pService.AddHandlerFunc(thandler, func(ctx context.Context, invoice *lnrpc.Invoice, data string) error {
		if data != tdata {
			t.Errorf("Data expected %s, got %s", tdata, data)
		}
		if tinv.Memo != invoice.Memo {
			t.Errorf("invoice expected %s, got %s", tinv.Memo, invoice.Memo)
		}
		return nil
	})
	mockLnd.SettleInvoice(tinv)
	err = pService.RecoverInvoices(context.Background())
	if err != nil {
		t.Fatal(err)
	}

}

func randomHex(n int) (string, []byte, error) {
	bytes := make([]byte, n)
	if _, err := rand.Read(bytes); err != nil {
		return "", nil, err
	}
	return hex.EncodeToString(bytes), bytes, nil
}
