package tracker

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/coreos/bbolt"
	"strconv"
)

var (
	bountyIssuesBucket = []byte("bounty_issues")
	ErrDoesNotExist         = fmt.Errorf("does not exist")
)

type BountyIssueStore struct {
	db *bbolt.DB
}

func (store *BountyIssueStore) Add(ctx context.Context, issue *BountyIssue) error {
	tx, err := store.db.Begin(true)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	b := tx.Bucket(bountyIssuesBucket)
	if b == nil {
		return fmt.Errorf("bucket nil")
	}
	jData, err := json.Marshal(issue)
	if err != nil {
		return err
	}

	if err := b.Put([]byte(strconv.Itoa(int(issue.Id))), jData); err != nil {
		return err
	}

	return tx.Commit()
}

func (store *BountyIssueStore) Update(ctx context.Context, issue *BountyIssue) error {
	tx, err := store.db.Begin(true)
	if err != nil {
		return  err
	}
	defer tx.Rollback()

	b := tx.Bucket(bountyIssuesBucket)
	if b == nil {
		return fmt.Errorf("bucket nil")
	}

	jData := b.Get([]byte(strconv.Itoa(int(issue.Id))))
	if jData == nil {
		return ErrDoesNotExist
	}
	jData, err = json.Marshal(issue)
	if err != nil {
		return err
	}
	if err := b.Put([]byte(strconv.Itoa(int(issue.Id))), jData); err != nil {
		return err
	}
	return tx.Commit()
}

func (store *BountyIssueStore) Get(ctx context.Context, id int64) (*BountyIssue, error) {
	tx, err := store.db.Begin(false)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	b := tx.Bucket(bountyIssuesBucket)
	if b == nil {
		return nil, fmt.Errorf("bucket nil")
	}

	jData := b.Get([]byte(strconv.Itoa(int(id))))
	if jData == nil {
		return nil, ErrDoesNotExist
	}

	unhandledPayment := &BountyIssue{}
	if err := json.Unmarshal(jData, unhandledPayment); err != nil {
		return nil, err
	}

	return unhandledPayment, nil
}

func (store *BountyIssueStore) Delete(ctx context.Context, id int64) error {
	tx, err := store.db.Begin(true)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	b := tx.Bucket(bountyIssuesBucket)
	if b == nil {
		return fmt.Errorf("bucket nil")
	}

	if err := b.Delete([]byte(strconv.Itoa(int(id)))); err != nil {
		return err
	}

	return tx.Commit()
}

func NewBountyIssueStore(db *bbolt.DB) (*BountyIssueStore, error) {
	tx, err := db.Begin(true)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	_, err = tx.CreateBucketIfNotExists(bountyIssuesBucket)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &BountyIssueStore{db: db}, nil
}