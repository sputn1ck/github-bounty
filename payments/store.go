package payments

import (
	"encoding/json"
	"fmt"
	"github.com/coreos/bbolt"
)

var (
	unhandledpaymentsbucket = []byte("unhandled_payments")
	ErrDoesNotExist         = fmt.Errorf("does not exist")
)

type unhandledPaymentsStore struct {
	db *bbolt.DB
}

func (u *unhandledPaymentsStore) ListAll() ([]*UnhandledPayment, error) {
	tx, err := u.db.Begin(false)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	b := tx.Bucket(unhandledpaymentsbucket)
	if b == nil {
		return nil, fmt.Errorf("bucket nil")
	}
	var payments []*UnhandledPayment
	err = b.ForEach(func(k, v []byte) error {

		payment := &UnhandledPayment{}
		if err := json.Unmarshal(v, payment); err != nil {
			return err
		}
		payments = append(payments, payment)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return payments, nil
}

func newUnhandledPaymentsStore(db *bbolt.DB) (*unhandledPaymentsStore, error) {
	tx, err := db.Begin(true)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	_, err = tx.CreateBucketIfNotExists(unhandledpaymentsbucket)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &unhandledPaymentsStore{db: db}, nil
}

func (u *unhandledPaymentsStore) Add(payment *UnhandledPayment) error {
	tx, err := u.db.Begin(true)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	b := tx.Bucket(unhandledpaymentsbucket)
	if b == nil {
		return fmt.Errorf("bucket nil")
	}
	jData, err := json.Marshal(payment)
	if err != nil {
		return err
	}

	if err := b.Put([]byte(payment.RHash), jData); err != nil {
		return err
	}

	return tx.Commit()
}

func (u *unhandledPaymentsStore) Get(rHash string) (*UnhandledPayment, error) {
	tx, err := u.db.Begin(false)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	b := tx.Bucket(unhandledpaymentsbucket)
	if b == nil {
		return nil, fmt.Errorf("bucket nil")
	}

	jData := b.Get([]byte(rHash))
	if jData == nil {
		return nil, ErrDoesNotExist
	}

	unhandledPayment := &UnhandledPayment{}
	if err := json.Unmarshal(jData, unhandledPayment); err != nil {
		return nil, err
	}

	return unhandledPayment, nil
}

func (u *unhandledPaymentsStore) Remove(rHash string) error {
	tx, err := u.db.Begin(true)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	b := tx.Bucket(unhandledpaymentsbucket)
	if b == nil {
		return fmt.Errorf("bucket nil")
	}

	if err := b.Delete([]byte(rHash)); err != nil {
		return err
	}

	return tx.Commit()
}
