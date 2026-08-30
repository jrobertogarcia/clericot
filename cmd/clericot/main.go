package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"clericot/internal/config"
	"clericot/internal/platform/database"
	"clericot/sql"
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "clericot",
		Short: "Clericot Enterprise Framework Developer Tooling & CLI",
	}

	// Module Scaffolding Subcommands
	moduleCmd := &cobra.Command{
		Use:   "module",
		Short: "Domain module generator and scaffolding commands",
	}

	moduleCreateCmd := &cobra.Command{
		Use:   "create [name]",
		Short: "Scaffold a new enterprise domain module with the canonical 6-file layout (entity, repo, service, handler, worker, module)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := strings.ToLower(args[0])
			return scaffoldModule(name)
		},
	}
	moduleCmd.AddCommand(moduleCreateCmd)

	// Migration Subcommands
	migrateCmd := &cobra.Command{
		Use:   "migrate",
		Short: "Database migration management commands",
	}

	migrateCreateCmd := &cobra.Command{
		Use:   "create [name]",
		Short: "Create a new timestamped Goose SQL migration file",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := strings.ToLower(args[0])
			timestamp := time.Now().Format("20060102150405")
			filename := fmt.Sprintf("sql/migrations/%s_%s.sql", timestamp, name)
			content := `-- +goose Up
-- SQL in section 'Up' is executed when this migration is applied

-- +goose Down
-- SQL section 'Down' is executed when this migration is rolled back
`
			if err := os.WriteFile(filename, []byte(content), 0644); err != nil {
				return fmt.Errorf("failed to create migration file: %w", err)
			}
			fmt.Printf("Created migration file: %s\n", filename)
			return nil
		},
	}

	migrateUpCmd := &cobra.Command{
		Use:   "up",
		Short: "Apply all pending database migrations",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			cfg, err := config.Load()
			if err != nil {
				return err
			}

			pool, err := database.NewPool(ctx, cfg.Database)
			if err != nil {
				return err
			}
			defer pool.Close()

			database.SetMigrationsFS(sql.MigrationsFS)
			if err := database.MigrateUp(ctx, pool, "migrations"); err != nil {
				return fmt.Errorf("failed to apply migrations: %w", err)
			}

			fmt.Println("All migrations applied successfully.")
			return nil
		},
	}

	migrateCmd.AddCommand(migrateCreateCmd, migrateUpCmd)
	rootCmd.AddCommand(moduleCmd, migrateCmd)

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func scaffoldModule(name string) error {
	capitalized := strings.ToUpper(name[:1]) + name[1:]
	dir := filepath.Join("internal", "modules", name)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	// 1. entity.go (Pure domain entities & sentinels, zero external persistence/transport imports)
	entityContent := fmt.Sprintf(`package %s

import (
	"errors"
	"time"
)

type %s struct {
	ID        string
	TenantID  string
	Name      string
	CreatedAt time.Time
	UpdatedAt time.Time
}

var (
	Err%sNotFound = errors.New("%s not found")
)
`, name, capitalized, capitalized, name)
	if err := os.WriteFile(filepath.Join(dir, "entity.go"), []byte(entityContent), 0644); err != nil {
		return err
	}

	// 2. repository.go (Data access layer)
	repoContent := fmt.Sprintf(`package %s

import (
	"context"
	"clericot/internal/platform/database"
)

type Repository struct {
	txManager *database.TxManager
}

func NewRepository(txManager *database.TxManager) *Repository {
	return &Repository{txManager: txManager}
}
`, name)
	if err := os.WriteFile(filepath.Join(dir, "repository.go"), []byte(repoContent), 0644); err != nil {
		return err
	}

	// 3. service.go (Domain business logic)
	serviceContent := fmt.Sprintf(`package %s

import (
	"context"
	"clericot/internal/platform/database"
)

type Service struct {
	repo      *Repository
	txManager *database.TxManager
}

func NewService(repo *Repository, txManager *database.TxManager) *Service {
	return &Service{
		repo:      repo,
		txManager: txManager,
	}
}
`, name)
	if err := os.WriteFile(filepath.Join(dir, "service.go"), []byte(serviceContent), 0644); err != nil {
		return err
	}

	// 4. handler.go (Huma v2 typed HTTP endpoints)
	handlerContent := fmt.Sprintf(`package %s

import (
	"context"
	"github.com/danielgtaylor/huma/v2"
)

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) RegisterRoutes(api huma.API) {
	// Register typed Huma OpenAPI operations
}
`, name)
	if err := os.WriteFile(filepath.Join(dir, "handler.go"), []byte(handlerContent), 0644); err != nil {
		return err
	}

	// 5. worker.go (River / Asynq background event handlers)
	workerContent := fmt.Sprintf(`package %s

import (
	"context"
	"log/slog"
)
`, name)
	if err := os.WriteFile(filepath.Join(dir, "worker.go"), []byte(workerContent), 0644); err != nil {
		return err
	}

	// 6. module.go (Explicit constructor DI wiring)
	moduleContent := fmt.Sprintf(`package %s

import (
	"github.com/danielgtaylor/huma/v2"
	"clericot/internal/platform/database"
)

type Module struct {
	Handler *Handler
	Service *Service
	Repo    *Repository
}

func NewModule(api huma.API, txManager *database.TxManager) *Module {
	repo := NewRepository(txManager)
	service := NewService(repo, txManager)
	handler := NewHandler(service)

	if api != nil {
		handler.RegisterRoutes(api)
	}

	return &Module{
		Handler: handler,
		Service: service,
		Repo:    repo,
	}
}
`, name)
	if err := os.WriteFile(filepath.Join(dir, "module.go"), []byte(moduleContent), 0644); err != nil {
		return err
	}

	fmt.Printf("Successfully scaffolded domain module: internal/modules/%s (6-file anatomy)\n", name)
	return nil
}
