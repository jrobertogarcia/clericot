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

	// 1. entity.go (Pure domain entities, value objects & sentinels, zero persistence/transport imports)
	entityContent := fmt.Sprintf(`package %s

import (
	"errors"
	"time"
)

// %s represents a pure domain entity decoupled from persistence models.
type %s struct {
	ID        string
	TenantID  string
	Name      string
	Status    string
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt *time.Time
}

// Create%sDTO contains input data required to create a new %s.
type Create%sDTO struct {
	TenantID string
	Name     string
}

// Domain error sentinels.
var (
	Err%sNotFound = errors.New("%s not found")
	Err%sRequired = errors.New("%s data is required")
	ErrTenantRequired = errors.New("tenant_id is required")
)
`, name, capitalized, capitalized, capitalized, name, capitalized, capitalized, name, capitalized, name)
	if err := os.WriteFile(filepath.Join(dir, "entity.go"), []byte(entityContent), 0644); err != nil {
		return err
	}

	// 2. repository.go (Data access layer with TxManager)
	repoContent := fmt.Sprintf(`package %s

import (
	"context"
	"time"

	"clericot/internal/platform/database"
)

// Repository handles database persistence operations for the %s domain.
type Repository struct {
	txManager *database.TxManager
}

// NewRepository constructs a new %s Repository instance.
func NewRepository(txManager *database.TxManager) *Repository {
	return &Repository{
		txManager: txManager,
	}
}

// Create persists a new %s entity.
func (r *Repository) Create(ctx context.Context, e *%s) (*%s, error) {
	db := r.txManager.GetDB(ctx)
	now := time.Now().UTC()
	if e.CreatedAt.IsZero() {
		e.CreatedAt = now
	}
	e.UpdatedAt = now

	// Persistence implementation (sqlc or Bob dynamic builder)
	_ = db
	return e, nil
}

// GetByID retrieves a %s entity by its ID and tenant ID.
func (r *Repository) GetByID(ctx context.Context, tenantID, id string) (*%s, error) {
	db := r.txManager.GetDB(ctx)
	_ = db
	_ = tenantID
	_ = id
	return nil, Err%sNotFound
}
`, name, name, capitalized, capitalized, capitalized, capitalized, capitalized, capitalized, capitalized)
	if err := os.WriteFile(filepath.Join(dir, "repository.go"), []byte(repoContent), 0644); err != nil {
		return err
	}

	// 3. service.go (Domain business logic & transaction orchestration via RunInTx)
	serviceContent := fmt.Sprintf(`package %s

import (
	"context"
	"time"

	"github.com/google/uuid"

	"clericot/internal/platform/database"
	"clericot/internal/platform/tenant"
)

// Service encapsulates domain business logic for the %s domain.
type Service struct {
	repo      *Repository
	txManager *database.TxManager
}

// NewService constructs a new %s Service instance.
func NewService(repo *Repository, txManager *database.TxManager) *Service {
	return &Service{
		repo:      repo,
		txManager: txManager,
	}
}

// Create processes creation of a new %s entity within a tenant transaction.
func (s *Service) Create(ctx context.Context, dto Create%sDTO) (*%s, error) {
	if dto.TenantID == "" {
		return nil, ErrTenantRequired
	}

	entity := &%s{
		ID:        uuid.NewString(),
		TenantID:  dto.TenantID,
		Name:      dto.Name,
		Status:    "active",
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}

	var created *%s
	err := s.txManager.RunInTx(tenant.WithTenant(ctx, dto.TenantID), func(txCtx context.Context) error {
		var err error
		created, err = s.repo.Create(txCtx, entity)
		return err
	})
	if err != nil {
		return nil, err
	}

	return created, nil
}

// GetByID retrieves a %s entity by ID within a tenant transaction.
func (s *Service) GetByID(ctx context.Context, tenantID, id string) (*%s, error) {
	if tenantID == "" {
		return nil, ErrTenantRequired
	}

	var result *%s
	err := s.txManager.RunInTx(tenant.WithTenant(ctx, tenantID), func(txCtx context.Context) error {
		var err error
		result, err = s.repo.GetByID(txCtx, tenantID, id)
		return err
	})
	if err != nil {
		return nil, err
	}

	return result, nil
}
`, name, name, capitalized, capitalized, capitalized, capitalized, capitalized, capitalized, capitalized, capitalized, capitalized)
	if err := os.WriteFile(filepath.Join(dir, "service.go"), []byte(serviceContent), 0644); err != nil {
		return err
	}

	// 4. handler.go (Huma v2 typed HTTP endpoints & RFC 9457 Problem Details error mapping)
	handlerContent := fmt.Sprintf(`package %s

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"clericot/internal/platform/httperr"
	"clericot/internal/platform/tenant"
)

// Create%sInput defines HTTP request parameters for creating a %s.
type Create%sInput struct {
	Body struct {
		TenantID string `+"`"+`json:"tenant_id" doc:"Tenant identifier" example:"tenant_123"`+"`"+`
		Name     string `+"`"+`json:"name" doc:"%s name" minLength:"2" example:"Sample %s"`+"`"+`
	}
}

// %sResponse defines the HTTP response payload.
type %sResponse struct {
	Body struct {
		ID        string    `+"`"+`json:"id" doc:"Unique identifier"`+"`"+`
		TenantID  string    `+"`"+`json:"tenant_id" doc:"Tenant ID"`+"`"+`
		Name      string    `+"`"+`json:"name" doc:"Name"`+"`"+`
		Status    string    `+"`"+`json:"status" doc:"Status"`+"`"+`
		CreatedAt time.Time `+"`"+`json:"created_at" doc:"Creation timestamp"`+"`"+`
	}
}

// Handler handles HTTP transport operations for the %s domain.
type Handler struct {
	svc *Service
}

// NewHandler constructs a new %s Handler instance.
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// RegisterRoutes registers Huma v2 typed OpenAPI operations.
func (h *Handler) RegisterRoutes(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "create-%s",
		Method:      http.MethodPost,
		Path:        "/v1/%s",
		Summary:     "Create %s",
		Description: "Creates a new %s within the active tenant.",
		Tags:        []string{"%s"},
	}, func(ctx context.Context, input *Create%sInput) (*%sResponse, error) {
		tenantID := input.Body.TenantID
		if tenantID == "" {
			tenantID = tenant.FromContext(ctx)
		}

		res, err := h.svc.Create(ctx, Create%sDTO{
			TenantID: tenantID,
			Name:     input.Body.Name,
		})
		if err != nil {
			return nil, mapDomainError(err)
		}

		resp := &%sResponse{}
		resp.Body.ID = res.ID
		resp.Body.TenantID = res.TenantID
		resp.Body.Name = res.Name
		resp.Body.Status = res.Status
		resp.Body.CreatedAt = res.CreatedAt
		return resp, nil
	})
}

func mapDomainError(err error) error {
	if err == nil {
		return nil
	}

	var prob *httperr.Problem
	if errors.As(err, &prob) {
		return prob
	}

	switch {
	case errors.Is(err, Err%sNotFound):
		return httperr.NewNotFound(err.Error())
	case errors.Is(err, ErrTenantRequired):
		return httperr.NewBadRequest(err.Error())
	default:
		return httperr.Transform(err)
	}
}
`, name, capitalized, name, capitalized, capitalized, capitalized, capitalized, capitalized, name, capitalized, name, name, capitalized, name, capitalized, capitalized, capitalized, capitalized, capitalized, capitalized)
	if err := os.WriteFile(filepath.Join(dir, "handler.go"), []byte(handlerContent), 0644); err != nil {
		return err
	}

	// 5. worker.go (River outbox event workers & Asynq background task handlers)
	workerContent := fmt.Sprintf(`package %s

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/hibiken/asynq"
	"github.com/riverqueue/river"
)

const (
	// Type%sCreated defines the Asynq task type for %s events.
	Type%sCreated = "%s:created"
)

// %sCreatedJobArgs defines River outbox job arguments for %s.
type %sCreatedJobArgs struct {
	ID       string `+"`"+`json:"id"`+"`"+`
	TenantID string `+"`"+`json:"tenant_id"`+"`"+`
}

// Kind implements river.JobArgs.
func (%sCreatedJobArgs) Kind() string {
	return "%s.created.v1"
}

// %sCreatedWorker processes River background events for %s.
type %sCreatedWorker struct {
	river.WorkerDefaults[%sCreatedJobArgs]
	logger *slog.Logger
}

// New%sCreatedWorker constructs a new %s worker.
func New%sCreatedWorker(logger *slog.Logger) *%sCreatedWorker {
	if logger == nil {
		logger = slog.Default()
	}
	return &%sCreatedWorker{logger: logger}
}

// Work handles the execution of %sCreatedJobArgs.
func (w *%sCreatedWorker) Work(ctx context.Context, job *river.Job[%sCreatedJobArgs]) error {
	w.logger.Info("processing %s created event",
		"id", job.Args.ID,
		"tenant_id", job.Args.TenantID,
	)
	return nil
}

// Process%sTask handles Asynq background tasks.
func Process%sTask(ctx context.Context, t *asynq.Task) error {
	var payload struct {
		ID       string `+"`"+`json:"id"`+"`"+`
		TenantID string `+"`"+`json:"tenant_id"`+"`"+`
	}
	if err := json.Unmarshal(t.Payload(), &payload); err != nil {
		return fmt.Errorf("failed to parse %s task payload: %%w", err)
	}

	slog.Info("processing asynq task for %s", "id", payload.ID, "tenant_id", payload.TenantID)
	return nil
}
`, name, capitalized, name, capitalized, name, capitalized, name, capitalized, capitalized, name, capitalized, name, capitalized, capitalized, capitalized, name, capitalized, capitalized, capitalized, capitalized, capitalized, capitalized, name, capitalized, capitalized, name, name)
	if err := os.WriteFile(filepath.Join(dir, "worker.go"), []byte(workerContent), 0644); err != nil {
		return err
	}

	// 6. module.go (Explicit constructor dependency injection wiring)
	moduleContent := fmt.Sprintf(`package %s

import (
	"github.com/danielgtaylor/huma/v2"

	"clericot/internal/platform/database"
)

// Module encapsulates the wired %s domain module components.
type Module struct {
	Handler *Handler
	Service *Service
	Repo    *Repository
}

// NewModule constructs and wires the %s domain module, registering HTTP routes with Huma.
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
`, name, name, capitalized)
	if err := os.WriteFile(filepath.Join(dir, "module.go"), []byte(moduleContent), 0644); err != nil {
		return err
	}

	fmt.Printf("Successfully scaffolded domain module: internal/modules/%s (6-file anatomy)\n", name)
	return nil
}
