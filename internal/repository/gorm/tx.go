package gorm

import (
	"context"

	"gorm.io/gorm"
)

type txKey struct{}

// TxRunner executes functions inside a database transaction.
type TxRunner struct {
	db *gorm.DB
}

// NewTxRunner creates a new TxRunner with the provided DB handle.
func NewTxRunner(db *gorm.DB) *TxRunner {
	return &TxRunner{db: db}
}

// Run executes fn inside a transaction. The context passed to fn carries the active transaction.
func (r *TxRunner) Run(ctx context.Context, fn func(ctx context.Context) error) error {
	if ctx == nil {
		ctx = context.Background()
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		txCtx := context.WithValue(ctx, txKey{}, tx)
		return fn(txCtx)
	})
}

// DBFrom returns the active transaction *gorm.DB from ctx if present, or fallback.WithContext(ctx) if fallback is non-nil.
func DBFrom(ctx context.Context, fallback *gorm.DB) *gorm.DB {
	if ctx != nil {
		if tx, ok := ctx.Value(txKey{}).(*gorm.DB); ok && tx != nil {
			return tx
		}
		if fallback != nil {
			return fallback.WithContext(ctx)
		}
	}
	return fallback
}
