package orders_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ordersModule "clericot/internal/modules/orders"
	platformAuth "clericot/internal/platform/auth"
	"clericot/tests/fixtures"
	"clericot/tests/testsuite"
)

var (
	testOrdersMod *ordersModule.Module
)

func TestMain(m *testing.M) {
	testsuite.Main(m)
}

func TestOrdersModule_LifecycleAndSearch(t *testing.T) {
	ctx := context.Background()
	testOrdersMod = ordersModule.NewModule(nil, testsuite.SharedTxManager, nil)

	// 1. Setup Tenant & User
	tenantID, err := testsuite.SeedTenant(ctx, "Orders Module Tenant")
	require.NoError(t, err)

	user, err := testsuite.SeedUser(ctx, tenantID, "buyer-"+uuid.NewString()[:6]+"@orders-mod.com", "Order Buyer", "member")
	require.NoError(t, err)

	principal := &platformAuth.AuthPrincipal{
		ID:       user.ID,
		TenantID: tenantID,
		Email:    user.Email,
		Role:     "member",
	}
	authedCtx := platformAuth.WithPrincipal(ctx, principal)

	// 2. Create Order 1 with synthetic items
	itemDTO1 := fixtures.NewCreateOrderItemDTO(
		fixtures.WithCreateOrderItemProductName("MacBook Pro"),
		fixtures.WithCreateOrderItemQuantity(1),
		fixtures.WithCreateOrderItemUnitPriceCents(200000),
	)
	itemDTO2 := fixtures.NewCreateOrderItemDTO(
		fixtures.WithCreateOrderItemProductName("Magic Mouse"),
		fixtures.WithCreateOrderItemQuantity(2),
		fixtures.WithCreateOrderItemUnitPriceCents(8000),
	)
	items1 := []ordersModule.CreateOrderItemDTO{itemDTO1, itemDTO2}

	ord1, err := testOrdersMod.Service.CreateOrder(authedCtx, items1)
	require.NoError(t, err)
	require.NotNil(t, ord1)
	assert.Equal(t, int64(216000), ord1.TotalCents) // 200000 + 16000
	assert.Equal(t, ordersModule.OrderStatusPending, ord1.Status)
	assert.Len(t, ord1.Items, 2)

	// 3. Create Order 2
	itemDTO3 := fixtures.NewCreateOrderItemDTO(
		fixtures.WithCreateOrderItemProductName("USB-C Cable"),
		fixtures.WithCreateOrderItemQuantity(3),
		fixtures.WithCreateOrderItemUnitPriceCents(1500),
	)
	items2 := []ordersModule.CreateOrderItemDTO{itemDTO3}

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
	tenantB, err := testsuite.SeedTenant(ctx, "Other Tenant Org")
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
