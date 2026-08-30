package main_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"clericot/internal/config"
	"clericot/internal/modules/auth"
	"clericot/internal/modules/orders"
	"clericot/internal/platform/app"
	platformAuth "clericot/internal/platform/auth"
	"clericot/internal/platform/database"
	"clericot/internal/platform/router"
	"clericot/internal/platform/tenant"
	"clericot/internal/sqlcgen"
	"clericot/sql"
)

var (
	testAdminPool *pgxpool.Pool
	testAppPool   *pgxpool.Pool
	bundle        *router.RouterBundle
)

func TestMain(m *testing.M) {
	ctx := context.Background()

	pgContainer, err := postgres.Run(ctx,
		"postgres:16-alpine",
		postgres.WithDatabase("testdb"),
		postgres.WithUsername("postgres"),
		postgres.WithPassword("postgrespassword"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(30*time.Second)),
	)
	if err != nil {
		panic(err)
	}

	adminConnStr, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		panic(err)
	}

	testAdminPool, err = database.NewPool(ctx, config.DatabaseConfig{
		URL:             adminConnStr,
		MaxConns:        10,
		MinConns:        2,
		MaxConnLifetime: 5 * time.Minute,
	})
	if err != nil {
		panic(err)
	}

	database.SetMigrationsFS(sql.MigrationsFS)
	if err := database.MigrateUp(ctx, testAdminPool, "migrations"); err != nil {
		panic(err)
	}

	appConnStr := strings.Replace(adminConnStr, "postgres:postgrespassword", "app_user:app_user_password", 1)
	testAppPool, err = database.NewPool(ctx, config.DatabaseConfig{
		URL:             appConnStr,
		MaxConns:        10,
		MinConns:        2,
		MaxConnLifetime: 5 * time.Minute,
	})
	if err != nil {
		panic(err)
	}

	cfg, _ := config.Load()
	txManager := database.NewTxManager(testAppPool)
	tokenService := platformAuth.NewTokenService(cfg.Auth.JWTSecret, nil)

	healthChecker := app.NewHealthChecker(testAppPool, nil)
	bundle = router.NewRouter(cfg, healthChecker, tokenService.HTTPMiddleware, tenant.Middleware)

	// Explicit constructor DI for domain modules
	auth.NewModule(bundle.API, txManager, tokenService)
	orders.NewModule(bundle.API, txManager, nil)

	code := m.Run()

	testAppPool.Close()
	testAdminPool.Close()
	_ = pgContainer.Terminate(ctx)

	os.Exit(code)
}

func TestAPI_E2EAuthAndOrderFlow(t *testing.T) {
	ctx := context.Background()
	adminQueries := sqlcgen.New(testAdminPool)
	ts := pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}

	// 1. Create Tenant
	tenantID := "tenant-e2e-" + uuid.NewString()[:8]
	_, err := adminQueries.CreateTenant(ctx, sqlcgen.CreateTenantParams{
		ID:        tenantID,
		Name:      "E2E API Test Corp",
		Status:    "active",
		CreatedAt: ts,
		UpdatedAt: ts,
	})
	require.NoError(t, err)

	// 2. Register User via HTTP POST /v1/auth/register
	regPayload, _ := json.Marshal(map[string]any{
		"tenant_id": tenantID,
		"email":     "e2e-api-user@corp.com",
		"password":  "SuperSecret2026!",
		"name":      "E2E API Tester",
	})
	regReq := httptest.NewRequest("POST", "/v1/auth/register", bytes.NewReader(regPayload))
	regReq.Header.Set("Content-Type", "application/json")
	regRec := httptest.NewRecorder()
	bundle.Mux.ServeHTTP(regRec, regReq)

	assert.Equal(t, http.StatusOK, regRec.Code)
	var authResp struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
	}
	err = json.Unmarshal(regRec.Body.Bytes(), &authResp)
	require.NoError(t, err)
	require.NotEmpty(t, authResp.AccessToken)

	// 3. Login User via HTTP POST /v1/auth/login
	loginPayload, _ := json.Marshal(map[string]any{
		"tenant_id": tenantID,
		"email":     "e2e-api-user@corp.com",
		"password":  "SuperSecret2026!",
	})
	loginReq := httptest.NewRequest("POST", "/v1/auth/login", bytes.NewReader(loginPayload))
	loginReq.Header.Set("Content-Type", "application/json")
	loginRec := httptest.NewRecorder()
	bundle.Mux.ServeHTTP(loginRec, loginReq)
	assert.Equal(t, http.StatusOK, loginRec.Code)

	// 4. Create Order via HTTP POST /v1/orders
	orderPayload, _ := json.Marshal(map[string]any{
		"items": []map[string]any{
			{
				"product_name":     "Widget Pro",
				"quantity":         2,
				"unit_price_cents": 1500,
			},
		},
	})
	orderReq := httptest.NewRequest("POST", "/v1/orders", bytes.NewReader(orderPayload))
	orderReq.Header.Set("Content-Type", "application/json")
	orderReq.Header.Set("Authorization", "Bearer "+authResp.AccessToken)
	orderRec := httptest.NewRecorder()
	bundle.Mux.ServeHTTP(orderRec, orderReq)
	assert.Equal(t, http.StatusOK, orderRec.Code)

	var ordResp struct {
		ID         string `json:"id"`
		TotalCents int64  `json:"total_cents"`
		Status     string `json:"status"`
	}
	err = json.Unmarshal(orderRec.Body.Bytes(), &ordResp)
	require.NoError(t, err)
	assert.NotEmpty(t, ordResp.ID)
	assert.Equal(t, int64(3000), ordResp.TotalCents)
	assert.Equal(t, "pending", ordResp.Status)

	// 5. Search Orders via HTTP GET /v1/orders/search
	searchReq := httptest.NewRequest("GET", "/v1/orders/search?status=pending", nil)
	searchReq.Header.Set("Authorization", "Bearer "+authResp.AccessToken)
	searchRec := httptest.NewRecorder()
	bundle.Mux.ServeHTTP(searchRec, searchReq)
	assert.Equal(t, http.StatusOK, searchRec.Code)

	var searchResp struct {
		Orders []struct {
			ID         string `json:"id"`
			TotalCents int64  `json:"total_cents"`
			Status     string `json:"status"`
		} `json:"orders"`
		Count int `json:"count"`
	}
	err = json.Unmarshal(searchRec.Body.Bytes(), &searchResp)
	require.NoError(t, err)
	assert.Equal(t, 1, searchResp.Count)
	assert.Equal(t, ordResp.ID, searchResp.Orders[0].ID)
}
