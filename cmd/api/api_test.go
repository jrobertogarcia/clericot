package main_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"clericot/internal/config"
	"clericot/internal/modules/auth"
	"clericot/internal/modules/orders"
	"clericot/internal/platform/app"
	platformAuth "clericot/internal/platform/auth"
	"clericot/internal/platform/router"
	"clericot/internal/platform/tenant"
	"clericot/tests/fixtures"
	"clericot/tests/testsuite"
)

var (
	bundle *router.RouterBundle
)

func TestMain(m *testing.M) {
	testsuite.Main(m)
}

func setupAPIRouter(t *testing.T) *router.RouterBundle {
	t.Helper()

	cfg, _ := config.Load()
	tokenService := platformAuth.NewTokenService("e2e-api-jwt-secret-key-32-chars-long", nil)

	healthChecker := app.NewHealthChecker(testsuite.SharedAppPool, nil)
	b := router.NewRouter(cfg, healthChecker, tokenService.HTTPMiddleware, tenant.Middleware)

	// Explicit constructor DI for domain modules
	auth.NewModule(b.API, testsuite.SharedTxManager, tokenService)
	orders.NewModule(b.API, testsuite.SharedTxManager, nil)

	return b
}

func TestAPI_E2EAuthAndOrderFlow(t *testing.T) {
	ctx := context.Background()
	bundle = setupAPIRouter(t)

	// 1. Create Tenant
	tenantID, err := testsuite.SeedTenant(ctx, "E2E API Test Corp")
	require.NoError(t, err)

	regDTO := fixtures.NewRegisterDTO(
		fixtures.WithRegisterTenantID(tenantID),
		fixtures.WithRegisterEmail("e2e-user-"+uuid.NewString()[:6]+"@corp.com"),
		fixtures.WithRegisterName("E2E API Tester"),
		fixtures.WithRegisterPassword("SuperSecret2026!"),
	)

	// 2. Register User via HTTP POST /v1/auth/register
	regPayload, _ := json.Marshal(map[string]any{
		"tenant_id": regDTO.TenantID,
		"email":     regDTO.Email,
		"password":  regDTO.Password,
		"name":      regDTO.Name,
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
		"tenant_id": regDTO.TenantID,
		"email":     regDTO.Email,
		"password":  regDTO.Password,
	})
	loginReq := httptest.NewRequest("POST", "/v1/auth/login", bytes.NewReader(loginPayload))
	loginReq.Header.Set("Content-Type", "application/json")
	loginRec := httptest.NewRecorder()
	bundle.Mux.ServeHTTP(loginRec, loginReq)
	assert.Equal(t, http.StatusOK, loginRec.Code)

	// 4. Create Order via HTTP POST /v1/orders
	itemDTO := fixtures.NewCreateOrderItemDTO(
		fixtures.WithCreateOrderItemProductName("Widget Pro"),
		fixtures.WithCreateOrderItemQuantity(2),
		fixtures.WithCreateOrderItemUnitPriceCents(1500),
	)
	orderPayload, _ := json.Marshal(map[string]any{
		"items": []map[string]any{
			{
				"product_name":     itemDTO.ProductName,
				"quantity":         itemDTO.Quantity,
				"unit_price_cents": itemDTO.UnitPriceCents,
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
