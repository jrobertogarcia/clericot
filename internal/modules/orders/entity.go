package orders

import (
	"errors"
	"time"
)

// OrderStatus defines the lifecycle state of an order.
type OrderStatus string

const (
	OrderStatusDraft     OrderStatus = "draft"
	OrderStatusPending   OrderStatus = "pending"
	OrderStatusPaid      OrderStatus = "paid"
	OrderStatusCancelled OrderStatus = "cancelled"
	OrderStatusCompleted OrderStatus = "completed"
)

// OrderItem represents a line item in an order.
type OrderItem struct {
	ID             string
	OrderID        string
	TenantID       string
	ProductName    string
	Quantity       int
	UnitPriceCents int64
	CreatedAt      time.Time
}

// Order represents an order domain aggregate entity.
type Order struct {
	ID         string
	TenantID   string
	UserID     string
	TotalCents int64
	Status     OrderStatus
	Items      []*OrderItem
	CreatedAt  time.Time
	UpdatedAt  time.Time
	DeletedAt  *time.Time
}

// CreateOrderItemDTO represents input data for creating an order item.
type CreateOrderItemDTO struct {
	ProductName    string
	Quantity       int
	UnitPriceCents int64
}

// SearchFilter represents multi-predicate search criteria for querying orders.
type SearchFilter struct {
	Status         *OrderStatus
	UserID         *string
	MinAmountCents *int64
	MaxAmountCents *int64
	Limit          int32
	Offset         int32
}

// Domain error sentinels.
var (
	ErrOrderNotFound         = errors.New("order not found")
	ErrOrderAlreadyCancelled = errors.New("order is already cancelled")
	ErrInvalidOrderItems     = errors.New("order must contain at least one valid line item")
	ErrUnauthenticated       = errors.New("unauthenticated request")
)
