package orders

import (
	"github.com/danielgtaylor/huma/v2"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	"clericot/internal/platform/database"
)

// Module encapsulates the wired orders domain module components.
type Module struct {
	Handler *Handler
	Service *Service
	Repo    *Repository
}

// NewModule constructs and wires the orders domain module, registering HTTP routes with Huma.
func NewModule(api huma.API, txManager *database.TxManager, riverClient *river.Client[pgx.Tx]) *Module {
	repo := NewRepository(txManager)
	service := NewService(repo, txManager, riverClient)
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
