package orders_test

import (
	"context"
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
	ordersModule "clericot/internal/modules/orders"
	platformAuth "clericot/internal/platform/auth"
	"clericot/internal/platform/database"
	"clericot/internal/platform/tenant"
	"clericot/internal/sqlcgen"
	"clericot/sql"
)

var (
	testAdminPool *pgxpool.Pool
	testAppPool   *pgxpool.Pool
	testOrdersMod *ordersModule.Module
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

	txManager := database.NewTxManager(testAppPool)
	testOrdersMod = ordersModule.NewModule(nil, txManager, nil)

	code := m.Run()

	testAppPool.Close()
	testAdminPool.Close()
	_ = pgContainer.Terminate(ctx)

	os.Exit(code)
}

func TestOrdersModule_LifecycleAndSearch(t *testing.T) {
	ctx := context.Background()
	adminQueries := sqlcgen.New(testAdminPool)
	ts := pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}

	// 1. Setup Tenant & User
	tenantID := "tenant-mod-ord-" + uuid.NewString()[:8]
	_, err := adminQueries.CreateTenant(ctx, sqlcgen.CreateTenantParams{
		ID:        tenantID,
		Name:      "Orders Module Tenant",
		Status:    "active",
		CreatedAt: ts,
		UpdatedAt: ts,
	})
	require.NoError(t, err)

	userID := "usr-ord-" + uuid.NewString()[:8]
	txManager := database.NewTxManager(testAppPool)
	err = txManager.RunInTx(tenant.WithTenant(ctx, tenantID), func(txCtx context.Context) error {
		db := txManager.GetDB(txCtx)
		_, err := sqlcgen.New(db).CreateUser(txCtx, sqlcgen.CreateUserParams{
			ID:           userID,
			TenantID:     tenantID,
			Email:        "buyer@orders-mod.com",
			Name:         "Order Buyer",
			PasswordHash: "hashed_pw",
			Role:         "member",
			CreatedAt:    ts,
			UpdatedAt:    ts,
		})
		return err
	})
	require.NoError(t, err)

	principal := &platformAuth.AuthPrincipal{
		ID:       userID,
		TenantID: tenantID,
		Email:    "buyer@orders-mod.com",
		Role:     "member",
	}
	authedCtx := platformAuth.WithPrincipal(ctx, principal)

	// 2. Create Order 1
	items1 := []ordersModule.CreateOrderItemDTO{
		{ProductName: "MacBook Pro", Quantity: 1, UnitPriceCents: 200000},
		{ProductName: "Magic Mouse", Quantity: 2, UnitPriceCents: 8000},
	}
	ord1, err := testOrdersMod.Service.CreateOrder(authedCtx, items1)
	require.NoError(t, err)
	require.NotNil(t, ord1)
	assert.Equal(t, int64(216000), ord1.TotalCents) // 200000 + 16000
	assert.Equal(t, ordersModule.OrderStatusPending, ord1.Status)
	assert.Len(t, ord1.Items, 2)

	// 3. Create Order 2
	items2 := []ordersModule.CreateOrderItemDTO{
		{ProductName: "USB-C Cable", Quantity: 3, UnitPriceCents: 1500},
	}
	ord2, err := testOrdersMod.Service.CreateOrder(authedCtx, items2)
	require.NoError(t, err)
	require.NotNil(t, ord2)
	assert.Equal(t, int64(4500), ord2.TotalCents)

	// 4. Get By ID
	fetched, err := testOrdersMod.Service.GetOrderByID(authedCtx, ord1.ID)
	require.NoError(t, err)
	assert.Equal(t, ord1.ID, fetched.ID)
	assert.Len(t, fetched.Items, 2)

	// 5. Cancel Order 2
	cancelled, err := testOrdersMod.Service.CancelOrder(authedCtx, ord2.ID)
	require.NoError(t, err)
	assert.Equal(t, ordersModule.OrderStatusCancelled, cancelled.Status)

	// Cancelling again should return ErrOrderAlreadyCancelled
	_, err = testOrdersMod.Service.CancelOrder(authedCtx, ord2.ID)
	assert.ErrorIs(t, err, ordersModule.ErrOrderAlreadyCancelled)

	// 6. Dynamic Bob Search Tests
	// Search by Status: pending
	pendingStatus := ordersModule.OrderStatusPending
	pendingResults, err := testOrdersMod.Service.SearchOrders(authedCtx, ordersModule.SearchFilter{
		Status: &pendingStatus,
	})
	require.NoError(t, err)
	assert.Len(t, pendingResults, 1)
	assert.Equal(t, ord1.ID, pendingResults[0].ID)

	// Search by Status: cancelled
	cancelledStatus := ordersModule.OrderStatusCancelled
	cancelledResults, err := testOrdersMod.Service.SearchOrders(authedCtx, ordersModule.SearchFilter{
		Status: &cancelledStatus,
	})
	require.NoError(t, err)
	assert.Len(t, cancelledResults, 1)
	assert.Equal(t, ord2.ID, cancelledResults[0].ID)

	// Search by MinAmountCents: >= 100000
	minAmount := int64(100000)
	expensiveResults, err := testOrdersMod.Service.SearchOrders(authedCtx, ordersModule.SearchFilter{
		MinAmountCents: &minAmount,
	})
	require.NoError(t, err)
	assert.Len(t, expensiveResults, 1)
	assert.Equal(t, ord1.ID, expensiveResults[0].ID)

	// Search by MaxAmountCents: <= 10000
	maxAmount := int64(10000)
	cheapResults, err := testOrdersMod.Service.SearchOrders(authedCtx, ordersModule.SearchFilter{
		MaxAmountCents: &maxAmount,
	})
	require.NoError(t, err)
	assert.Len(t, cheapResults, 1)
	assert.Equal(t, ord2.ID, cheapResults[0].ID)

	// Search with Pagination: limit 1
	paginatedResults, err := testOrdersMod.Service.SearchOrders(authedCtx, ordersModule.SearchFilter{
		Limit: 1,
	})
	require.NoError(t, err)
	assert.Len(t, paginatedResults, 1)

	// 7. Multi-Tenant RLS Isolation
	tenantB := "tenant-mod-other-" + uuid.NewString()[:8]
	_, err = adminQueries.CreateTenant(ctx, sqlcgen.CreateTenantParams{
		ID:        tenantB,
		Name:      "Other Tenant Org",
		Status:    "active",
		CreatedAt: ts,
		UpdatedAt: ts,
	})
	require.NoError(t, err)

	principalB := &platformAuth.AuthPrincipal{
		ID:       "usr-other-b",
		TenantID: tenantB,
		Email:    "other@other.com",
		Role:     "member",
	}
	authedCtxB := platformAuth.WithPrincipal(ctx, principalB)

	// Tenant B cannot get Order 1
	_, err = testOrdersMod.Service.GetOrderByID(authedCtxB, ord1.ID)
	assert.ErrorIs(t, err, ordersModule.ErrOrderNotFound)

	// Tenant B search returns empty
	searchB, err := testOrdersMod.Service.SearchOrders(authedCtxB, ordersModule.SearchFilter{})
	require.NoError(t, err)
	assert.Empty(t, searchB)
}
