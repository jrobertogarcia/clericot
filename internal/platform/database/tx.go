package database

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type txKey struct{}

// TenantSetter is an optional callback function to configure tenant session on transaction start.
var TenantSetter func(ctx context.Context, tx pgx.Tx) error

// DBTX is the common database executor interface satisfied by *pgxpool.Pool, *pgxpool.Conn, and pgx.Tx.
type DBTX interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// TxManager coordinates context-bound transactions with savepoint support and detached rollback.
type TxManager struct {
	pool *pgxpool.Pool
}

// NewTxManager constructs a new TxManager.
func NewTxManager(pool *pgxpool.Pool) *TxManager {
	return &TxManager{pool: pool}
}

// RunInTx executes a function inside an atomic database transaction.
// If an active transaction already exists in ctx, it creates a SAVEPOINT pseudo-nested transaction.
func (m *TxManager) RunInTx(ctx context.Context, fn func(txCtx context.Context) error) error {
	// 1. Handle Nested Transaction via PostgreSQL SAVEPOINT
	if parentTx, ok := ctx.Value(txKey{}).(pgx.Tx); ok {
		nestedTx, err := parentTx.Begin(ctx) // Creates SAVEPOINT in pgx v5
		if err != nil {
			return fmt.Errorf("failed to create savepoint: %w", err)
		}

		var committed bool
		defer func() {
			if !committed {
				// Detached rollback context ensures savepoint rollback executes even if ctx is canceled
				rollbackCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
				defer cancel()
				_ = nestedTx.Rollback(rollbackCtx)
			}
		}()

		nestedCtx := context.WithValue(ctx, txKey{}, nestedTx)
		if err := fn(nestedCtx); err != nil {
			return err // Trigger savepoint rollback
		}

		if err := nestedTx.Commit(ctx); err != nil { // Releases SAVEPOINT
			return fmt.Errorf("failed to release savepoint: %w", err)
		}
		committed = true
		return nil
	}

	// 2. Begin Root Transaction
	tx, err := m.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin root tx: %w", err)
	}

	var committed bool
	defer func() {
		if !committed {
			// Detach cancellation to guarantee database connection cleanup
			rollbackCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
			defer cancel()
			_ = tx.Rollback(rollbackCtx)
		}
	}()

	// 3. Inject Tenant Context into PostgreSQL Session if hook is registered
	if TenantSetter != nil {
		if err := TenantSetter(ctx, tx); err != nil {
			return fmt.Errorf("failed to set tenant session config: %w", err)
		}
	}

	txCtx := context.WithValue(ctx, txKey{}, tx)
	if err := fn(txCtx); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit root tx: %w", err)
	}
	committed = true
	return nil
}

// WithTx returns a context carrying the provided transaction.
func WithTx(ctx context.Context, tx pgx.Tx) context.Context {
	return context.WithValue(ctx, txKey{}, tx)
}

// GetDB returns the active transaction from context, or the base connection pool.
func (m *TxManager) GetDB(ctx context.Context) DBTX {
	if tx, ok := ctx.Value(txKey{}).(pgx.Tx); ok {
		return tx
	}
	return m.pool
}

// GetTx returns the active pgx.Tx from context (required for River.InsertTx).
func (m *TxManager) GetTx(ctx context.Context) pgx.Tx {
	if tx, ok := ctx.Value(txKey{}).(pgx.Tx); ok {
		return tx
	}
	return nil
}
