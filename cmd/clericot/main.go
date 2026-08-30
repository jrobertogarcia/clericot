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
		Short: "Scaffold a new enterprise domain module with models, service, handlers, and queries",
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
	dir := filepath.Join("internal", "domain", name)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	// 1. model.go
	modelContent := fmt.Sprintf(`package %s

type %sInput struct {
	Body struct {
		Name string `+"`"+`json:"name" doc:"Entity name"`+"`"+`
	}
}

type %sResponse struct {
	Body struct {
		ID   string `+"`"+`json:"id"`+"`"+`
		Name string `+"`"+`json:"name"`+"`"+`
	}
}
`, name, capitalized, capitalized)
	if err := os.WriteFile(filepath.Join(dir, "model.go"), []byte(modelContent), 0644); err != nil {
		return err
	}

	// 2. service.go
	serviceContent := fmt.Sprintf(`package %s

import (
	"context"
	"clericot/internal/platform/database"
)

type %sService struct {
	txManager *database.TxManager
}

func New%sService(txManager *database.TxManager) *%sService {
	return &%sService{txManager: txManager}
}
`, name, capitalized, capitalized, capitalized, capitalized)
	if err := os.WriteFile(filepath.Join(dir, "service.go"), []byte(serviceContent), 0644); err != nil {
		return err
	}

	// 3. handler.go
	handlerContent := fmt.Sprintf(`package %s

import (
	"github.com/danielgtaylor/huma/v2"
)

func RegisterRoutes(api huma.API, svc *%sService) {
	// Register typed Huma OpenAPI routes
}
`, name, capitalized)
	if err := os.WriteFile(filepath.Join(dir, "handler.go"), []byte(handlerContent), 0644); err != nil {
		return err
	}

	fmt.Printf("Successfully scaffolded domain module: internal/domain/%s\n", name)
	return nil
}
