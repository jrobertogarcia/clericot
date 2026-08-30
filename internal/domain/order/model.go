package order

import "time"

type OrderItemInput struct {
	ProductName    string `json:"product_name" doc:"Product or service name" minLength:"1"`
	Quantity       int    `json:"quantity" doc:"Quantity ordered" minimum:"1"`
	UnitPriceCents int64  `json:"unit_price_cents" doc:"Unit price in integer cents" minimum:"0"`
}

type CreateOrderInput struct {
	Body struct {
		Items []OrderItemInput `json:"items" doc:"List of line items in the order" minItems:"1"`
	}
}

type OrderIDParam struct {
	ID string `path:"id" doc:"Unique order identifier"`
}

type OrderItemResponse struct {
	ID             string `json:"id"`
	ProductName    string `json:"product_name"`
	Quantity       int    `json:"quantity"`
	UnitPriceCents int64  `json:"unit_price_cents"`
}

type OrderResponse struct {
	Body struct {
		ID         string              `json:"id"`
		TenantID   string              `json:"tenant_id"`
		UserID     string              `json:"user_id"`
		TotalCents int64               `json:"total_cents"`
		Status     string              `json:"status"`
		Items      []OrderItemResponse `json:"items"`
		CreatedAt  time.Time           `json:"created_at"`
		UpdatedAt  time.Time           `json:"updated_at"`
	}
}
