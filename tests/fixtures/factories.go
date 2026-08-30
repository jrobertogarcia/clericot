package fixtures

import (
	"time"

	"github.com/brianvoe/gofakeit/v7"

	"clericot/internal/modules/auth"
	"clericot/internal/modules/orders"
)

func init() {
	// Seed gofakeit with a deterministic constant for reproducible synthetic fixtures
	_ = gofakeit.Seed(42)
}

// ResetSeed resets the internal gofakeit seed.
func ResetSeed(seed uint64) {
	_ = gofakeit.Seed(seed)
}

// ==========================================
// Auth Module Entity Factories
// ==========================================

// UserOption defines a functional mutator for auth.User domain entities.
type UserOption func(*auth.User)

// WithUserID overrides the User ID.
func WithUserID(id string) UserOption {
	return func(u *auth.User) { u.ID = id }
}

// WithUserTenantID overrides the User TenantID.
func WithUserTenantID(tenantID string) UserOption {
	return func(u *auth.User) { u.TenantID = tenantID }
}

// WithUserEmail overrides the User Email.
func WithUserEmail(email string) UserOption {
	return func(u *auth.User) { u.Email = email }
}

// WithUserName overrides the User Name.
func WithUserName(name string) UserOption {
	return func(u *auth.User) { u.Name = name }
}

// WithUserPasswordHash overrides the User PasswordHash.
func WithUserPasswordHash(hash string) UserOption {
	return func(u *auth.User) { u.PasswordHash = hash }
}

// WithUserRole overrides the User Role.
func WithUserRole(role auth.Role) UserOption {
	return func(u *auth.User) { u.Role = role }
}

// WithUserCreatedAt overrides the User CreatedAt timestamp.
func WithUserCreatedAt(t time.Time) UserOption {
	return func(u *auth.User) { u.CreatedAt = t }
}

// WithUserUpdatedAt overrides the User UpdatedAt timestamp.
func WithUserUpdatedAt(t time.Time) UserOption {
	return func(u *auth.User) { u.UpdatedAt = t }
}

// NewUser constructs a synthetic auth.User domain model with sensible deterministic defaults.
func NewUser(opts ...UserOption) *auth.User {
	now := time.Now().UTC()
	u := &auth.User{
		ID:           "usr-" + gofakeit.UUID()[:8],
		TenantID:     "tenant-" + gofakeit.UUID()[:8],
		Email:        gofakeit.Email(),
		Name:         gofakeit.Name(),
		PasswordHash: "hashed_" + gofakeit.Password(true, true, true, false, false, 12),
		Role:         auth.RoleMember,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	for _, opt := range opts {
		opt(u)
	}
	return u
}

// TenantOption defines a functional mutator for auth.Tenant domain entities.
type TenantOption func(*auth.Tenant)

// WithTenantID overrides the Tenant ID.
func WithTenantID(id string) TenantOption {
	return func(t *auth.Tenant) { t.ID = id }
}

// WithTenantName overrides the Tenant Name.
func WithTenantName(name string) TenantOption {
	return func(t *auth.Tenant) { t.Name = name }
}

// WithTenantStatus overrides the Tenant Status.
func WithTenantStatus(status string) TenantOption {
	return func(t *auth.Tenant) { t.Status = status }
}

// WithTenantCreatedAt overrides the Tenant CreatedAt timestamp.
func WithTenantCreatedAt(tm time.Time) TenantOption {
	return func(t *auth.Tenant) { t.CreatedAt = tm }
}

// WithTenantUpdatedAt overrides the Tenant UpdatedAt timestamp.
func WithTenantUpdatedAt(tm time.Time) TenantOption {
	return func(t *auth.Tenant) { t.UpdatedAt = tm }
}

// WithTenantDeletedAt overrides the Tenant DeletedAt timestamp.
func WithTenantDeletedAt(tm *time.Time) TenantOption {
	return func(t *auth.Tenant) { t.DeletedAt = tm }
}

// NewTenant constructs a synthetic auth.Tenant domain model.
func NewTenant(opts ...TenantOption) *auth.Tenant {
	now := time.Now().UTC()
	t := &auth.Tenant{
		ID:        "tenant-" + gofakeit.UUID()[:8],
		Name:      gofakeit.Company(),
		Status:    "active",
		CreatedAt: now,
		UpdatedAt: now,
	}

	for _, opt := range opts {
		opt(t)
	}
	return t
}

// RegisterDTOOption defines a functional mutator for auth.RegisterDTO.
type RegisterDTOOption func(*auth.RegisterDTO)

// WithRegisterTenantID overrides RegisterDTO TenantID.
func WithRegisterTenantID(tenantID string) RegisterDTOOption {
	return func(d *auth.RegisterDTO) { d.TenantID = tenantID }
}

// WithRegisterEmail overrides RegisterDTO Email.
func WithRegisterEmail(email string) RegisterDTOOption {
	return func(d *auth.RegisterDTO) { d.Email = email }
}

// WithRegisterPassword overrides RegisterDTO Password.
func WithRegisterPassword(password string) RegisterDTOOption {
	return func(d *auth.RegisterDTO) { d.Password = password }
}

// WithRegisterName overrides RegisterDTO Name.
func WithRegisterName(name string) RegisterDTOOption {
	return func(d *auth.RegisterDTO) { d.Name = name }
}

// NewRegisterDTO constructs a synthetic auth.RegisterDTO.
func NewRegisterDTO(opts ...RegisterDTOOption) auth.RegisterDTO {
	dto := auth.RegisterDTO{
		TenantID: "tenant-" + gofakeit.UUID()[:8],
		Email:    gofakeit.Email(),
		Password: "Pass" + gofakeit.Password(true, true, true, false, false, 10) + "!",
		Name:     gofakeit.Name(),
	}

	for _, opt := range opts {
		opt(&dto)
	}
	return dto
}

// LoginDTOOption defines a functional mutator for auth.LoginDTO.
type LoginDTOOption func(*auth.LoginDTO)

// WithLoginTenantID overrides LoginDTO TenantID.
func WithLoginTenantID(tenantID string) LoginDTOOption {
	return func(d *auth.LoginDTO) { d.TenantID = tenantID }
}

// WithLoginEmail overrides LoginDTO Email.
func WithLoginEmail(email string) LoginDTOOption {
	return func(d *auth.LoginDTO) { d.Email = email }
}

// WithLoginPassword overrides LoginDTO Password.
func WithLoginPassword(password string) LoginDTOOption {
	return func(d *auth.LoginDTO) { d.Password = password }
}

// NewLoginDTO constructs a synthetic auth.LoginDTO.
func NewLoginDTO(opts ...LoginDTOOption) auth.LoginDTO {
	dto := auth.LoginDTO{
		TenantID: "tenant-" + gofakeit.UUID()[:8],
		Email:    gofakeit.Email(),
		Password: "Pass" + gofakeit.Password(true, true, true, false, false, 10) + "!",
	}

	for _, opt := range opts {
		opt(&dto)
	}
	return dto
}

// ==========================================
// Orders Module Entity Factories
// ==========================================

// OrderItemOption defines a functional mutator for orders.OrderItem domain entities.
type OrderItemOption func(*orders.OrderItem)

// WithOrderItemID overrides the OrderItem ID.
func WithOrderItemID(id string) OrderItemOption {
	return func(item *orders.OrderItem) { item.ID = id }
}

// WithOrderItemOrderID overrides the OrderItem OrderID.
func WithOrderItemOrderID(orderID string) OrderItemOption {
	return func(item *orders.OrderItem) { item.OrderID = orderID }
}

// WithOrderItemTenantID overrides the OrderItem TenantID.
func WithOrderItemTenantID(tenantID string) OrderItemOption {
	return func(item *orders.OrderItem) { item.TenantID = tenantID }
}

// WithOrderItemProductName overrides the OrderItem ProductName.
func WithOrderItemProductName(name string) OrderItemOption {
	return func(item *orders.OrderItem) { item.ProductName = name }
}

// WithOrderItemQuantity overrides the OrderItem Quantity.
func WithOrderItemQuantity(qty int) OrderItemOption {
	return func(item *orders.OrderItem) { item.Quantity = qty }
}

// WithOrderItemUnitPriceCents overrides the OrderItem UnitPriceCents.
func WithOrderItemUnitPriceCents(cents int64) OrderItemOption {
	return func(item *orders.OrderItem) { item.UnitPriceCents = cents }
}

// WithOrderItemCreatedAt overrides the OrderItem CreatedAt timestamp.
func WithOrderItemCreatedAt(t time.Time) OrderItemOption {
	return func(item *orders.OrderItem) { item.CreatedAt = t }
}

// NewOrderItem constructs a synthetic orders.OrderItem model with sensible defaults.
func NewOrderItem(opts ...OrderItemOption) *orders.OrderItem {
	item := &orders.OrderItem{
		ID:             "item-" + gofakeit.UUID()[:8],
		OrderID:        "ord-" + gofakeit.UUID()[:8],
		TenantID:       "tenant-" + gofakeit.UUID()[:8],
		ProductName:    gofakeit.ProductName(),
		Quantity:       gofakeit.Number(1, 5),
		UnitPriceCents: int64(gofakeit.Number(500, 25000)),
		CreatedAt:      time.Now().UTC(),
	}

	for _, opt := range opts {
		opt(item)
	}
	return item
}

// OrderOption defines a functional mutator for orders.Order domain entities.
type OrderOption func(*orders.Order)

// WithOrderID overrides the Order ID.
func WithOrderID(id string) OrderOption {
	return func(o *orders.Order) { o.ID = id }
}

// WithOrderTenantID overrides the Order TenantID.
func WithOrderTenantID(tenantID string) OrderOption {
	return func(o *orders.Order) { o.TenantID = tenantID }
}

// WithOrderUserID overrides the Order UserID.
func WithOrderUserID(userID string) OrderOption {
	return func(o *orders.Order) { o.UserID = userID }
}

// WithOrderTotalCents overrides the Order TotalCents.
func WithOrderTotalCents(cents int64) OrderOption {
	return func(o *orders.Order) { o.TotalCents = cents }
}

// WithOrderStatus overrides the Order Status.
func WithOrderStatus(status orders.OrderStatus) OrderOption {
	return func(o *orders.Order) { o.Status = status }
}

// WithOrderItems overrides the Order Items slice and recomputes total cents if not set.
func WithOrderItems(items ...*orders.OrderItem) OrderOption {
	return func(o *orders.Order) {
		o.Items = items
		var sum int64
		for _, it := range items {
			sum += int64(it.Quantity) * it.UnitPriceCents
		}
		o.TotalCents = sum
	}
}

// WithOrderCreatedAt overrides the Order CreatedAt timestamp.
func WithOrderCreatedAt(t time.Time) OrderOption {
	return func(o *orders.Order) { o.CreatedAt = t }
}

// WithOrderUpdatedAt overrides the Order UpdatedAt timestamp.
func WithOrderUpdatedAt(t time.Time) OrderOption {
	return func(o *orders.Order) { o.UpdatedAt = t }
}

// WithOrderDeletedAt overrides the Order DeletedAt timestamp.
func WithOrderDeletedAt(t *time.Time) OrderOption {
	return func(o *orders.Order) { o.DeletedAt = t }
}

// NewOrder constructs a synthetic orders.Order aggregate root.
func NewOrder(opts ...OrderOption) *orders.Order {
	now := time.Now().UTC()
	ord := &orders.Order{
		ID:         "ord-" + gofakeit.UUID()[:8],
		TenantID:   "tenant-" + gofakeit.UUID()[:8],
		UserID:     "usr-" + gofakeit.UUID()[:8],
		TotalCents: int64(gofakeit.Number(1000, 100000)),
		Status:     orders.OrderStatusPending,
		Items:      nil,
		CreatedAt:  now,
		UpdatedAt:  now,
	}

	for _, opt := range opts {
		opt(ord)
	}
	return ord
}

// CreateOrderItemDTOOption defines a functional mutator for orders.CreateOrderItemDTO.
type CreateOrderItemDTOOption func(*orders.CreateOrderItemDTO)

// WithCreateOrderItemProductName overrides CreateOrderItemDTO ProductName.
func WithCreateOrderItemProductName(name string) CreateOrderItemDTOOption {
	return func(d *orders.CreateOrderItemDTO) { d.ProductName = name }
}

// WithCreateOrderItemQuantity overrides CreateOrderItemDTO Quantity.
func WithCreateOrderItemQuantity(qty int) CreateOrderItemDTOOption {
	return func(d *orders.CreateOrderItemDTO) { d.Quantity = qty }
}

// WithCreateOrderItemUnitPriceCents overrides CreateOrderItemDTO UnitPriceCents.
func WithCreateOrderItemUnitPriceCents(cents int64) CreateOrderItemDTOOption {
	return func(d *orders.CreateOrderItemDTO) { d.UnitPriceCents = cents }
}

// NewCreateOrderItemDTO constructs a synthetic orders.CreateOrderItemDTO.
func NewCreateOrderItemDTO(opts ...CreateOrderItemDTOOption) orders.CreateOrderItemDTO {
	dto := orders.CreateOrderItemDTO{
		ProductName:    gofakeit.ProductName(),
		Quantity:       gofakeit.Number(1, 5),
		UnitPriceCents: int64(gofakeit.Number(500, 20000)),
	}

	for _, opt := range opts {
		opt(&dto)
	}
	return dto
}
