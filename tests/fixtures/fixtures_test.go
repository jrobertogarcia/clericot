package fixtures_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"clericot/internal/modules/auth"
	"clericot/internal/modules/orders"
	"clericot/tests/fixtures"
)

func TestFactories_NewUser(t *testing.T) {
	// Default synthetic user
	u1 := fixtures.NewUser()
	require.NotNil(t, u1)
	assert.NotEmpty(t, u1.ID)
	assert.NotEmpty(t, u1.TenantID)
	assert.NotEmpty(t, u1.Email)
	assert.NotEmpty(t, u1.Name)
	assert.Equal(t, auth.RoleMember, u1.Role)

	// User with options
	fixedTime := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	u2 := fixtures.NewUser(
		fixtures.WithUserID("usr-custom-1"),
		fixtures.WithUserTenantID("tenant-custom-1"),
		fixtures.WithUserEmail("custom@example.com"),
		fixtures.WithUserName("Custom Name"),
		fixtures.WithUserRole(auth.RoleAdmin),
		fixtures.WithUserPasswordHash("custom-hash"),
		fixtures.WithUserCreatedAt(fixedTime),
		fixtures.WithUserUpdatedAt(fixedTime),
	)
	assert.Equal(t, "usr-custom-1", u2.ID)
	assert.Equal(t, "tenant-custom-1", u2.TenantID)
	assert.Equal(t, "custom@example.com", u2.Email)
	assert.Equal(t, "Custom Name", u2.Name)
	assert.Equal(t, auth.RoleAdmin, u2.Role)
	assert.Equal(t, "custom-hash", u2.PasswordHash)
	assert.Equal(t, fixedTime, u2.CreatedAt)
	assert.Equal(t, fixedTime, u2.UpdatedAt)
}

func TestFactories_NewTenant(t *testing.T) {
	t1 := fixtures.NewTenant()
	require.NotNil(t, t1)
	assert.NotEmpty(t, t1.ID)
	assert.NotEmpty(t, t1.Name)
	assert.Equal(t, "active", t1.Status)

	t2 := fixtures.NewTenant(
		fixtures.WithTenantID("tenant-custom-2"),
		fixtures.WithTenantName("Custom Corp"),
		fixtures.WithTenantStatus("suspended"),
	)
	assert.Equal(t, "tenant-custom-2", t2.ID)
	assert.Equal(t, "Custom Corp", t2.Name)
	assert.Equal(t, "suspended", t2.Status)
}

func TestFactories_NewDTOs(t *testing.T) {
	reg := fixtures.NewRegisterDTO(
		fixtures.WithRegisterTenantID("t-1"),
		fixtures.WithRegisterEmail("reg@test.com"),
		fixtures.WithRegisterName("Reg Tester"),
		fixtures.WithRegisterPassword("SecretPass!"),
	)
	assert.Equal(t, "t-1", reg.TenantID)
	assert.Equal(t, "reg@test.com", reg.Email)
	assert.Equal(t, "Reg Tester", reg.Name)
	assert.Equal(t, "SecretPass!", reg.Password)

	login := fixtures.NewLoginDTO(
		fixtures.WithLoginTenantID("t-2"),
		fixtures.WithLoginEmail("login@test.com"),
		fixtures.WithLoginPassword("LoginPass!"),
	)
	assert.Equal(t, "t-2", login.TenantID)
	assert.Equal(t, "login@test.com", login.Email)
	assert.Equal(t, "LoginPass!", login.Password)
}

func TestFactories_NewOrderAndItems(t *testing.T) {
	item1 := fixtures.NewOrderItem(
		fixtures.WithOrderItemProductName("Item 1"),
		fixtures.WithOrderItemQuantity(2),
		fixtures.WithOrderItemUnitPriceCents(1000),
	)
	item2 := fixtures.NewOrderItem(
		fixtures.WithOrderItemProductName("Item 2"),
		fixtures.WithOrderItemQuantity(1),
		fixtures.WithOrderItemUnitPriceCents(500),
	)

	ord := fixtures.NewOrder(
		fixtures.WithOrderID("ord-custom-1"),
		fixtures.WithOrderStatus(orders.OrderStatusPaid),
		fixtures.WithOrderItems(item1, item2),
	)

	assert.Equal(t, "ord-custom-1", ord.ID)
	assert.Equal(t, orders.OrderStatusPaid, ord.Status)
	assert.Len(t, ord.Items, 2)
	assert.Equal(t, int64(2500), ord.TotalCents) // 2*1000 + 1*500

	dto := fixtures.NewCreateOrderItemDTO(
		fixtures.WithCreateOrderItemProductName("Gadget"),
		fixtures.WithCreateOrderItemQuantity(3),
		fixtures.WithCreateOrderItemUnitPriceCents(300),
	)
	assert.Equal(t, "Gadget", dto.ProductName)
	assert.Equal(t, 3, dto.Quantity)
	assert.Equal(t, int64(300), dto.UnitPriceCents)
}

func TestFactories_DeterministicSeeding(t *testing.T) {
	fixtures.ResetSeed(100)
	emailA1 := fixtures.NewUser().Email

	fixtures.ResetSeed(100)
	emailA2 := fixtures.NewUser().Email

	assert.Equal(t, emailA1, emailA2, "same seed should generate deterministic output")
}
