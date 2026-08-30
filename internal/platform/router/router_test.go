package router_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"clericot/internal/config"
	"clericot/internal/platform/app"
	"clericot/internal/platform/router"
)

func TestRouter_Endpoints(t *testing.T) {
	cfg, err := config.Load()
	require.NoError(t, err)

	healthChecker := app.NewHealthChecker(nil, nil)
	bundle := router.NewRouter(cfg, healthChecker)

	// 1. Test /livez probe
	req := httptest.NewRequest("GET", "/livez", nil)
	rec := httptest.NewRecorder()
	bundle.Mux.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "UP")

	// 2. Test /readyz probe
	req = httptest.NewRequest("GET", "/readyz", nil)
	rec = httptest.NewRecorder()
	bundle.Mux.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)

	// 3. Test OpenAPI JSON endpoint
	req = httptest.NewRequest("GET", "/openapi.json", nil)
	rec = httptest.NewRecorder()
	bundle.Mux.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "openapi")
}
