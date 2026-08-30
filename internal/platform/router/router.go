package router

import (
	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"

	"clericot/internal/config"
	"clericot/internal/platform/app"
)

// RouterBundle encapsulates the Chi router and Huma v2 typed API adapter.
type RouterBundle struct {
	Mux chi.Router
	API huma.API
}

// NewRouter constructs a production-ready Chi router and registers Huma v2 OpenAPI 3.1 engine.
func NewRouter(cfg *config.Config, healthChecker *app.HealthChecker) *RouterBundle {
	r := chi.NewRouter()

	// 1. Standard Security & Observability Middlewares
	r.Use(middleware.RequestID)
	r.Use(middleware.Recoverer)

	// 2. CORS Policy
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"https://*", "http://*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token", "X-Tenant-ID"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	// 3. Mount Asynchronous Health & Readiness Probes
	if healthChecker != nil {
		r.Mount("/livez", healthChecker.LiveHandler())
		r.Mount("/readyz", healthChecker.ReadyHandler())
	}

	// 4. Mount Huma v2 OpenAPI 3.1 Type-Safe API Engine
	humaConfig := huma.DefaultConfig(cfg.App.Name, "1.0.0")
	humaConfig.OpenAPIPath = "/openapi"
	humaConfig.DocsPath = "/docs"

	api := humachi.New(r, humaConfig)

	return &RouterBundle{
		Mux: r,
		API: api,
	}
}
