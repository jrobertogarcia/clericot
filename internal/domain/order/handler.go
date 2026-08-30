package order

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"clericot/internal/sqlcgen"
)

// RegisterRoutes registers Huma v2 typed OpenAPI operations for orders.
func RegisterRoutes(api huma.API, svc *OrderService) {
	huma.Register(api, huma.Operation{
		OperationID: "create-order",
		Method:      http.MethodPost,
		Path:        "/v1/orders",
		Summary:     "Create a new multi-item order",
		Tags:        []string{"Orders"},
	}, func(ctx context.Context, input *CreateOrderInput) (*OrderResponse, error) {
		ord, items, err := svc.CreateOrder(ctx, input.Body.Items)
		if err != nil {
			return nil, err
		}

		return mapOrderResponse(ord, items), nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-order-by-id",
		Method:      http.MethodGet,
		Path:        "/v1/orders/{id}",
		Summary:     "Retrieve an order by ID",
		Tags:        []string{"Orders"},
	}, func(ctx context.Context, input *OrderIDParam) (*OrderResponse, error) {
		ord, items, err := svc.GetOrderByID(ctx, input.ID)
		if err != nil {
			return nil, err
		}

		return mapOrderResponse(ord, items), nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "cancel-order",
		Method:      http.MethodPost,
		Path:        "/v1/orders/{id}/cancel",
		Summary:     "Cancel an existing order",
		Tags:        []string{"Orders"},
	}, func(ctx context.Context, input *OrderIDParam) (*OrderResponse, error) {
		ord, err := svc.CancelOrder(ctx, input.ID)
		if err != nil {
			return nil, err
		}

		return mapOrderResponse(ord, nil), nil
	})
}

func mapOrderResponse(ord *sqlcgen.Orders, items []sqlcgen.OrderItems) *OrderResponse {
	resp := &OrderResponse{}
	resp.Body.ID = ord.ID
	resp.Body.TenantID = ord.TenantID
	resp.Body.UserID = ord.UserID
	resp.Body.TotalCents = ord.TotalCents
	resp.Body.Status = ord.Status
	resp.Body.CreatedAt = ord.CreatedAt.Time
	resp.Body.UpdatedAt = ord.UpdatedAt.Time

	for _, item := range items {
		resp.Body.Items = append(resp.Body.Items, OrderItemResponse{
			ID:             item.ID,
			ProductName:    item.ProductName,
			Quantity:       int(item.Quantity),
			UnitPriceCents: item.UnitPriceCents,
		})
	}
	return resp
}
