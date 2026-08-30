package order_test

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
	domainOrder "clericot/internal/domain/order"
	platformAuth "clericot/internal/platform/auth"
	"clericot/internal/platform/database"
	"clericot/internal/platform/tenant"
	"clericot/internal/sqlcgen"
	"clericot/sql"
)

var (
	testAdminPool *pgxpool.Pool
	testAppPool   *pgxpool.Pool
	testOrderSvc  *domainOrder.OrderService
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
	testOrderSvc = domainOrder.NewOrderService(txManager, nil)

	code := m.Run()

	testAppPool.Close()
	testAdminPool.Close()
	_ = pgContainer.Terminate(ctx)

	os.Exit(code)
}

func TestOrderService_CreateGetAndCancel(t *testing.T) {
	ctx := context.Background()
	adminQueries := sqlcgen.New(testAdminPool)
	ts := pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}

	// 1. Create Tenant & User
	tenantID := "tenant-ord-" + uuid.NewString()[:8]
	_, err := adminQueries.CreateTenant(ctx, sqlcgen.CreateTenantParams{
		ID:        tenantID,
		Name:      "Order Tenant",
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
			Email:        "buyer@ord.com",
			Name:         "Buyer",
			PasswordHash: "pw",
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
		Email:    "buyer@ord.com",
		Role:     "member",
	}
	authedCtx := platformAuth.WithPrincipal(ctx, principal)

	// 2. Create Order
	items := []domainOrder.OrderItemInput{
		{ProductName: "Enterprise Subscription", Quantity: 1, UnitPriceCents: 50000},
		{ProductName: "Support Add-on", Quantity: 2, UnitPriceCents: 5000},
	}

	createdOrder, createdItems, err := testOrderSvc.CreateOrder(authedCtx, items)
	require.NoError(t, err)
	assert.Equal(t, int64(60000), createdOrder.TotalCents) // 50000 + (2 * 5000)
	assert.Equal(t, "pending", createdOrder.Status)
	assert.Len(t, createdItems, 2)

	// 3. Get Order by ID
	fetchedOrder, fetchedItems, err := testOrderSvc.GetOrderByID(authedCtx, createdOrder.ID)
	require.NoError(t, err)
	assert.Equal(t, createdOrder.ID, fetchedOrder.ID)
	assert.Len(t, fetchedItems, 2)

	// 4. Cancel Order
	cancelledOrder, err := testOrderSvc.CancelOrder(authedCtx, createdOrder.ID)
	require.NoError(t, err)
	assert.Equal(t, "cancelled", cancelledOrder.Status)

	// 5. Cross-Tenant Isolation: Tenant B cannot access this order
	tenantB := "tenant-other-" + uuid.NewString()[:8]
	_, err = adminQueries.CreateTenant(ctx, sqlcgen.CreateTenantParams{
		ID:        tenantB,
		Name:      "Other Tenant",
		Status:    "active",
		CreatedAt: ts,
		UpdatedAt: ts,
	})
	require.NoError(t, err)

	principalB := &platformAuth.AuthPrincipal{
		ID:       "usr-other",
		TenantID: tenantB,
		Email:    "other@other.com",
		Role:     "member",
	}
	authedCtxB := platformAuth.WithPrincipal(ctx, principalB)

	_, _, err = testOrderSvc.GetOrderByID(authedCtxB, createdOrder.ID)
	assert.Error(t, err) // RLS hides Tenant A's order from Tenant B!
}
