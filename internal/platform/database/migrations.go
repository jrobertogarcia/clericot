package database

import (
	"context"
	"embed"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

// MigrationsFS stores embedded SQL migration scripts.
var MigrationsFS embed.FS

// SetMigrationsFS configures the embedded migrations filesystem.
func SetMigrationsFS(fs embed.FS) {
	MigrationsFS = fs
	goose.SetBaseFS(MigrationsFS)
}

// MigrateUp executes pending Goose database migrations from the embedded filesystem.
func MigrateUp(ctx context.Context, pool *pgxpool.Pool, dir string) error {
	if dir == "" {
		dir = "sql/migrations"
	}

	db := stdlib.OpenDBFromPool(pool)
	defer db.Close()

	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("failed to set goose dialect: %w", err)
	}

	if err := goose.UpContext(ctx, db, dir); err != nil {
		return fmt.Errorf("failed to execute migrations up: %w", err)
	}

	return nil
}

// MigrateDown rolls back the most recent Goose migration.
func MigrateDown(ctx context.Context, pool *pgxpool.Pool, dir string) error {
	if dir == "" {
		dir = "sql/migrations"
	}

	db := stdlib.OpenDBFromPool(pool)
	defer db.Close()

	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("failed to set goose dialect: %w", err)
	}

	if err := goose.DownContext(ctx, db, dir); err != nil {
		return fmt.Errorf("failed to execute migrations down: %w", err)
	}

	return nil
}

// MigrateStatus reports the migration status.
func MigrateStatus(ctx context.Context, pool *pgxpool.Pool, dir string) error {
	if dir == "" {
		dir = "sql/migrations"
	}

	db := stdlib.OpenDBFromPool(pool)
	defer db.Close()

	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("failed to set goose dialect: %w", err)
	}

	if err := goose.StatusContext(ctx, db, dir); err != nil {
		return fmt.Errorf("failed to get migration status: %w", err)
	}

	return nil
}
