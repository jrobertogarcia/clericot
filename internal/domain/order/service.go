package order

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/riverqueue/river"

	"clericot/internal/platform/audit"
	platformAuth "clericot/internal/platform/auth"
	"clericot/internal/platform/database"
	"clericot/internal/platform/events"
	"clericot/internal/platform/httperr"
	"clericot/internal/platform/tenant"
	"clericot/internal/sqlcgen"
)

type OrderService struct {
	txManager   *database.TxManager
	riverClient *river.Client[pgx.Tx]
}

func NewOrderService(txManager *database.TxManager, riverClient *river.Client[pgx.Tx]) *OrderService {
	return &OrderService{
		txManager:   txManager,
		riverClient: riverClient,
	}
}

func (s *OrderService) CreateOrder(ctx context.Context, items []OrderItemInput) (*sqlcgen.Orders, []sqlcgen.OrderItems, error) {
	principal := platformAuth.PrincipalFromContext(ctx)
	if principal == nil {
		return nil, nil, httperr.NewUnauthorized("unauthenticated request")
	}

	orderID := uuid.NewString()
	ts := pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}

	var totalCents int64
	for _, it := range items {
		totalCents += int64(it.Quantity) * it.UnitPriceCents
	}

	var createdOrder sqlcgen.Orders
	var createdItems []sqlcgen.OrderItems

	err := s.txManager.RunInTx(tenant.WithTenant(ctx, principal.TenantID), func(txCtx context.Context) error {
		db := s.txManager.GetDB(txCtx)
		queries := sqlcgen.New(db)

		// 1. Insert Order Header
		ord, err := queries.CreateOrder(txCtx, sqlcgen.CreateOrderParams{
			ID:         orderID,
			TenantID:   principal.TenantID,
			UserID:     principal.ID,
			TotalCents: totalCents,
			Status:     "pending",
			CreatedAt:  ts,
			UpdatedAt:  ts,
		})
		if err != nil {
			return fmt.Errorf("failed to insert order: %w", err)
		}
		createdOrder = ord

		// 2. Insert Order Items
		for _, item := range items {
			lineID := uuid.NewString()
			ordItem, err := queries.CreateOrderItem(txCtx, sqlcgen.CreateOrderItemParams{
				ID:             lineID,
				OrderID:        orderID,
				TenantID:       principal.TenantID,
				ProductName:    item.ProductName,
				Quantity:       int32(item.Quantity),
				UnitPriceCents: item.UnitPriceCents,
				CreatedAt:      ts,
			})
			if err != nil {
				return fmt.Errorf("failed to insert order line item: %w", err)
			}
			createdItems = append(createdItems, ordItem)
		}

		// 3. Stage CloudEvent in River Outbox atomically
		eventPayload, _ := json.Marshal(map[string]any{
			"order_id":    orderID,
			"user_id":     principal.ID,
			"total_cents": totalCents,
			"status":      "pending",
		})

		domainEvt := events.DomainEvent{
			ID:        uuid.NewString(),
			Type:      "orders.order_created.v1",
			Source:    "order.service",
			TenantID:  principal.TenantID,
			Data:      eventPayload,
			Timestamp: time.Now().Unix(),
		}

		if s.riverClient != nil {
			tx := s.txManager.GetTx(txCtx)
			if tx != nil {
				if _, err := s.riverClient.InsertTx(txCtx, tx, domainEvt, nil); err != nil {
					return fmt.Errorf("failed to stage domain event in outbox: %w", err)
				}
			}
		}

		// 4. Stage Compliance Audit Log atomically
		auditPayload := audit.AuditPayload{
			ActorID:   principal.ID,
			TenantID:  principal.TenantID,
			Action:    "orders.create",
			Resource:  fmt.Sprintf("orders/%s", orderID),
			Diff:      eventPayload,
			Timestamp: time.Now().UTC(),
		}

		tx := s.txManager.GetTx(txCtx)
		if err := audit.StageAuditLog(txCtx, s.riverClient, tx, auditPayload); err != nil {
			return fmt.Errorf("failed to stage audit log: %w", err)
		}

		return nil
	})

	if err != nil {
		return nil, nil, httperr.Transform(err)
	}

	return &createdOrder, createdItems, nil
}

func (s *OrderService) GetOrderByID(ctx context.Context, id string) (*sqlcgen.Orders, []sqlcgen.OrderItems, error) {
	principal := platformAuth.PrincipalFromContext(ctx)
	if principal == nil {
		return nil, nil, httperr.NewUnauthorized("unauthenticated request")
	}

	var order sqlcgen.Orders
	var items []sqlcgen.OrderItems

	err := s.txManager.RunInTx(tenant.WithTenant(ctx, principal.TenantID), func(txCtx context.Context) error {
		db := s.txManager.GetDB(txCtx)
		queries := sqlcgen.New(db)

		ord, err := queries.GetOrderByID(txCtx, sqlcgen.GetOrderByIDParams{
			ID:       id,
			TenantID: principal.TenantID,
		})
		if err != nil {
			return httperr.NewNotFound("order not found")
		}
		order = ord

		its, err := queries.ListOrderItems(txCtx, sqlcgen.ListOrderItemsParams{
			OrderID:  id,
			TenantID: principal.TenantID,
		})
		if err != nil {
			return err
		}
		items = its
		return nil
	})

	if err != nil {
		return nil, nil, httperr.Transform(err)
	}

	return &order, items, nil
}

func (s *OrderService) CancelOrder(ctx context.Context, id string) (*sqlcgen.Orders, error) {
	principal := platformAuth.PrincipalFromContext(ctx)
	if principal == nil {
		return nil, httperr.NewUnauthorized("unauthenticated request")
	}

	ts := pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}
	var updatedOrder sqlcgen.Orders

	err := s.txManager.RunInTx(tenant.WithTenant(ctx, principal.TenantID), func(txCtx context.Context) error {
		db := s.txManager.GetDB(txCtx)
		queries := sqlcgen.New(db)

		ord, err := queries.GetOrderByID(txCtx, sqlcgen.GetOrderByIDParams{
			ID:       id,
			TenantID: principal.TenantID,
		})
		if err != nil {
			return httperr.NewNotFound("order not found")
		}

		if ord.Status == "cancelled" {
			return httperr.NewConflict("order is already cancelled")
		}

		upd, err := queries.UpdateOrderStatus(txCtx, sqlcgen.UpdateOrderStatusParams{
			ID:        id,
			TenantID:  principal.TenantID,
			Status:    "cancelled",
			UpdatedAt: ts,
		})
		if err != nil {
			return err
		}
		updatedOrder = upd

		// Stage audit log
		auditPayload := audit.AuditPayload{
			ActorID:   principal.ID,
			TenantID:  principal.TenantID,
			Action:    "orders.cancel",
			Resource:  fmt.Sprintf("orders/%s", id),
			Timestamp: time.Now().UTC(),
		}
		tx := s.txManager.GetTx(txCtx)
		_ = audit.StageAuditLog(txCtx, s.riverClient, tx, auditPayload)

		return nil
	})

	if err != nil {
		return nil, httperr.Transform(err)
	}

	return &updatedOrder, nil
}
