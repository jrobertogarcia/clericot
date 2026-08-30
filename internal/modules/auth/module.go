package auth

import (
	"github.com/danielgtaylor/huma/v2"

	platformAuth "clericot/internal/platform/auth"
	"clericot/internal/platform/database"
)

// Module encapsulates the wired auth domain module components.
type Module struct {
	Handler *Handler
	Service *Service
	Repo    *Repository
}

// NewModule constructs and wires the auth domain module, registering HTTP routes with Huma.
func NewModule(api huma.API, txManager *database.TxManager, tokenService *platformAuth.TokenService) *Module {
	repo := NewRepository(txManager)
	service := NewService(repo, txManager, tokenService)
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
