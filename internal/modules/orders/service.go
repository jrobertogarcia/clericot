package orders

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	"clericot/internal/platform/audit"
	platformAuth "clericot/internal/platform/auth"
	"clericot/internal/platform/database"
	"clericot/internal/platform/events"
	"clericot/internal/platform/tenant"
)

// Service coordinates business logic, transaction orchestration, outbox staging,
// and compliance audit logging for orders.
type Service struct {
	repo        *Repository
	txManager   *database.TxManager
	riverClient *river.Client[pgx.Tx]
}

// NewService constructs a new orders Service.
func NewService(repo *Repository, txManager *database.TxManager, riverClient *river.Client[pgx.Tx]) *Service {
	return &Service{
		repo:        repo,
		txManager:   txManager,
		riverClient: riverClient,
	}
}

// CreateOrder validates line items, calculates totals, inserts records in a transaction,
// and stages an atomic outbox event alongside an immutable audit trail.
func (s *Service) CreateOrder(ctx context.Context, items []CreateOrderItemDTO) (*Order, error) {
	principal := platformAuth.PrincipalFromContext(ctx)
	if principal == nil {
		return nil, ErrUnauthenticated
	}

	if len(items) == 0 {
		return nil, ErrInvalidOrderItems
	}

	var totalCents int64
	for _, it := range items {
		if it.Quantity <= 0 || it.UnitPriceCents < 0 {
			return nil, ErrInvalidOrderItems
		}
		totalCents += int64(it.Quantity) * it.UnitPriceCents
	}

	orderID := uuid.NewString()
	now := time.Now().UTC()

	var createdOrder *Order
	err := s.txManager.RunInTx(tenant.WithTenant(ctx, principal.TenantID), func(txCtx context.Context) error {
		// 1. Insert Order Header
		ord, err := s.repo.CreateOrder(txCtx, &Order{
			ID:         orderID,
			TenantID:   principal.TenantID,
			UserID:     principal.ID,
			TotalCents: totalCents,
			Status:     OrderStatusPending,
			CreatedAt:  now,
			UpdatedAt:  now,
		})
		if err != nil {
			return fmt.Errorf("failed to persist order: %w", err)
		}

		// 2. Insert Order Items
		var lineItems []*OrderItem
		for _, item := range items {
			lineID := uuid.NewString()
			createdItem, err := s.repo.CreateOrderItem(txCtx, &OrderItem{
				ID:             lineID,
				OrderID:        orderID,
				TenantID:       principal.TenantID,
				ProductName:    item.ProductName,
				Quantity:       item.Quantity,
				UnitPriceCents: item.UnitPriceCents,
				CreatedAt:      now,
			})
			if err != nil {
				return fmt.Errorf("failed to persist order item: %w", err)
			}
			lineItems = append(lineItems, createdItem)
		}
		ord.Items = lineItems
		createdOrder = ord

		// 3. Stage CloudEvent in River Outbox atomically
		eventPayload, err := json.Marshal(map[string]any{
			"order_id":    orderID,
			"user_id":     principal.ID,
			"total_cents": totalCents,
			"status":      string(OrderStatusPending),
		})
		if err != nil {
			return fmt.Errorf("failed to serialize domain event: %w", err)
		}

		domainEvt := events.DomainEvent{
			ID:        uuid.NewString(),
			Type:      "orders.order_created.v1",
			Source:    "orders.service",
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

		// 4. Stage Compliance Audit Log atomically (SOC 2 / HIPAA)
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
		return nil, err
	}

	return createdOrder, nil
}

// GetOrderByID retrieves an order with line items by its ID.
func (s *Service) GetOrderByID(ctx context.Context, id string) (*Order, error) {
	principal := platformAuth.PrincipalFromContext(ctx)
	if principal == nil {
		return nil, ErrUnauthenticated
	}

	var order *Order
	err := s.txManager.RunInTx(tenant.WithTenant(ctx, principal.TenantID), func(txCtx context.Context) error {
		ord, err := s.repo.GetByID(txCtx, principal.TenantID, id)
		if err != nil {
			return err
		}
		order = ord
		return nil
	})
	if err != nil {
		return nil, err
	}

	return order, nil
}

// CancelOrder transitions an order to cancelled status and stages an audit log.
func (s *Service) CancelOrder(ctx context.Context, id string) (*Order, error) {
	principal := platformAuth.PrincipalFromContext(ctx)
	if principal == nil {
		return nil, ErrUnauthenticated
	}

	var updatedOrder *Order
	err := s.txManager.RunInTx(tenant.WithTenant(ctx, principal.TenantID), func(txCtx context.Context) error {
		ord, err := s.repo.GetByID(txCtx, principal.TenantID, id)
		if err != nil {
			return err
		}

		if ord.Status == OrderStatusCancelled {
			return ErrOrderAlreadyCancelled
		}

		upd, err := s.repo.UpdateStatus(txCtx, principal.TenantID, id, OrderStatusCancelled)
		if err != nil {
			return err
		}
		upd.Items = ord.Items
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
		return nil, err
	}

	return updatedOrder, nil
}

// SearchOrders executes dynamic multi-predicate queries using Bob SQL builder.
func (s *Service) SearchOrders(ctx context.Context, filter SearchFilter) ([]*Order, error) {
	principal := platformAuth.PrincipalFromContext(ctx)
	if principal == nil {
		return nil, ErrUnauthenticated
	}

	var orders []*Order
	err := s.txManager.RunInTx(tenant.WithTenant(ctx, principal.TenantID), func(txCtx context.Context) error {
		res, err := s.repo.Search(txCtx, principal.TenantID, filter)
		if err != nil {
			return err
		}
		orders = res
		return nil
	})
	if err != nil {
		return nil, err
	}

	return orders, nil
}
