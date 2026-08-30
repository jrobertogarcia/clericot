package orders

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"clericot/internal/platform/httperr"
)

// OrderItemInput represents a line item in an order creation request.
type OrderItemInput struct {
	ProductName    string `json:"product_name" doc:"Product or service name" minLength:"1" example:"Enterprise Subscription"`
	Quantity       int    `json:"quantity" doc:"Quantity ordered" minimum:"1" example:"2"`
	UnitPriceCents int64  `json:"unit_price_cents" doc:"Unit price in integer cents" minimum:"0" example:"50000"`
}

// CreateOrderInput represents HTTP payload for creating a multi-item order.
type CreateOrderInput struct {
	Body struct {
		Items []OrderItemInput `json:"items" doc:"List of line items in the order" minItems:"1"`
	}
}

// OrderIDParam represents URL path parameter for single order operations.
type OrderIDParam struct {
	ID string `path:"id" doc:"Unique order identifier" example:"ord_01h8x..."`
}

// SearchOrdersInput represents query parameters for faceted dynamic order search.
type SearchOrdersInput struct {
	Status    string `query:"status" doc:"Filter by order status (draft, pending, paid, cancelled, completed)"`
	UserID    string `query:"user_id" doc:"Filter by user ID"`
	MinAmount int64  `query:"min_amount" doc:"Filter by minimum total amount in cents"`
	MaxAmount int64  `query:"max_amount" doc:"Filter by maximum total amount in cents"`
	Limit     int32  `query:"limit" doc:"Maximum number of records to return" default:"50"`
	Offset    int32  `query:"offset" doc:"Number of records to skip" default:"0"`
}

// OrderItemResponse represents serialized order item payload.
type OrderItemResponse struct {
	ID             string `json:"id" doc:"Unique line item ID"`
	ProductName    string `json:"product_name" doc:"Product name"`
	Quantity       int    `json:"quantity" doc:"Quantity ordered"`
	UnitPriceCents int64  `json:"unit_price_cents" doc:"Unit price in cents"`
}

// OrderResponseBody defines the body format of an order response.
type OrderResponseBody struct {
	ID         string              `json:"id" doc:"Unique order identifier"`
	TenantID   string              `json:"tenant_id" doc:"Tenant ID"`
	UserID     string              `json:"user_id" doc:"User ID"`
	TotalCents int64               `json:"total_cents" doc:"Total order price in cents"`
	Status     string              `json:"status" doc:"Order status"`
	Items      []OrderItemResponse `json:"items,omitempty" doc:"Line items"`
	CreatedAt  time.Time           `json:"created_at" doc:"Creation timestamp"`
	UpdatedAt  time.Time           `json:"updated_at" doc:"Last update timestamp"`
}

// OrderResponse represents single order HTTP response payload.
type OrderResponse struct {
	Body OrderResponseBody
}

// OrderListResponse represents a search results list of orders.
type OrderListResponse struct {
	Body struct {
		Orders []OrderResponseBody `json:"orders" doc:"Matching orders"`
		Count  int                 `json:"count" doc:"Total items returned"`
	}
}

// Handler handles HTTP transport operations for the orders domain.
type Handler struct {
	svc *Service
}

// NewHandler creates a new orders Handler.
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// RegisterRoutes registers Huma v2 typed OpenAPI operations for orders.
func (h *Handler) RegisterRoutes(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "create-order",
		Method:      http.MethodPost,
		Path:        "/v1/orders",
		Summary:     "Create a new multi-item order",
		Description: "Persists an order and line items inside a database transaction, staging an atomic outbox event.",
		Tags:        []string{"Orders"},
	}, func(ctx context.Context, input *CreateOrderInput) (*OrderResponse, error) {
		items := make([]CreateOrderItemDTO, len(input.Body.Items))
		for i, item := range input.Body.Items {
			items[i] = CreateOrderItemDTO(item)
		}

		ord, err := h.svc.CreateOrder(ctx, items)
		if err != nil {
			return nil, mapDomainError(err)
		}

		return &OrderResponse{Body: mapOrderToResponseBody(ord)}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-order-by-id",
		Method:      http.MethodGet,
		Path:        "/v1/orders/{id}",
		Summary:     "Retrieve an order by ID",
		Description: "Retrieves an order and its line items scoped strictly to the authenticated tenant.",
		Tags:        []string{"Orders"},
	}, func(ctx context.Context, input *OrderIDParam) (*OrderResponse, error) {
		ord, err := h.svc.GetOrderByID(ctx, input.ID)
		if err != nil {
			return nil, mapDomainError(err)
		}

		return &OrderResponse{Body: mapOrderToResponseBody(ord)}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "cancel-order",
		Method:      http.MethodPost,
		Path:        "/v1/orders/{id}/cancel",
		Summary:     "Cancel an existing order",
		Description: "Transitions order status to cancelled and creates an immutable audit trail.",
		Tags:        []string{"Orders"},
	}, func(ctx context.Context, input *OrderIDParam) (*OrderResponse, error) {
		ord, err := h.svc.CancelOrder(ctx, input.ID)
		if err != nil {
			return nil, mapDomainError(err)
		}

		return &OrderResponse{Body: mapOrderToResponseBody(ord)}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "search-orders",
		Method:      http.MethodGet,
		Path:        "/v1/orders/search",
		Summary:     "Dynamic multi-predicate order search",
		Description: "Searches orders using Bob dynamic SQL query builder preserving database index scans.",
		Tags:        []string{"Orders"},
	}, func(ctx context.Context, input *SearchOrdersInput) (*OrderListResponse, error) {
		var status *OrderStatus
		if input.Status != "" {
			st := OrderStatus(input.Status)
			status = &st
		}
		var userID *string
		if input.UserID != "" {
			userID = &input.UserID
		}
		var minAmount *int64
		if input.MinAmount > 0 {
			minAmount = &input.MinAmount
		}
		var maxAmount *int64
		if input.MaxAmount > 0 {
			maxAmount = &input.MaxAmount
		}

		filter := SearchFilter{
			Status:         status,
			UserID:         userID,
			MinAmountCents: minAmount,
			MaxAmountCents: maxAmount,
			Limit:          input.Limit,
			Offset:         input.Offset,
		}

		orders, err := h.svc.SearchOrders(ctx, filter)
		if err != nil {
			return nil, mapDomainError(err)
		}

		resp := &OrderListResponse{}
		resp.Body.Orders = make([]OrderResponseBody, len(orders))
		for i, o := range orders {
			resp.Body.Orders[i] = mapOrderToResponseBody(o)
		}
		resp.Body.Count = len(orders)
		return resp, nil
	})
}

func mapOrderToResponseBody(ord *Order) OrderResponseBody {
	b := OrderResponseBody{
		ID:         ord.ID,
		TenantID:   ord.TenantID,
		UserID:     ord.UserID,
		TotalCents: ord.TotalCents,
		Status:     string(ord.Status),
		CreatedAt:  ord.CreatedAt,
		UpdatedAt:  ord.UpdatedAt,
	}

	for _, item := range ord.Items {
		b.Items = append(b.Items, OrderItemResponse{
			ID:             item.ID,
			ProductName:    item.ProductName,
			Quantity:       item.Quantity,
			UnitPriceCents: item.UnitPriceCents,
		})
	}
	return b
}

func mapDomainError(err error) error {
	if err == nil {
		return nil
	}

	var prob *httperr.Problem
	if errors.As(err, &prob) {
		return prob
	}

	switch {
	case errors.Is(err, ErrOrderNotFound):
		return httperr.NewNotFound(err.Error())
	case errors.Is(err, ErrOrderAlreadyCancelled):
		return httperr.NewConflict(err.Error())
	case errors.Is(err, ErrInvalidOrderItems):
		return httperr.NewBadRequest(err.Error())
	case errors.Is(err, ErrUnauthenticated):
		return httperr.NewUnauthorized(err.Error())
	default:
		return httperr.Transform(err)
	}
}
