package orders

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stephenafamo/bob"
	"github.com/stephenafamo/bob/dialect/psql"
	"github.com/stephenafamo/bob/dialect/psql/dialect"
	"github.com/stephenafamo/bob/dialect/psql/sm"

	"clericot/internal/platform/database"
	"clericot/internal/sqlcgen"
)

// Repository handles database persistence operations for the orders domain.
type Repository struct {
	txManager *database.TxManager
}

// NewRepository constructs a new orders Repository.
func NewRepository(txManager *database.TxManager) *Repository {
	return &Repository{
		txManager: txManager,
	}
}

// CreateOrder inserts an order header into the database using sqlc.
func (r *Repository) CreateOrder(ctx context.Context, o *Order) (*Order, error) {
	db := r.txManager.GetDB(ctx)
	queries := sqlcgen.New(db)

	ts := pgtype.Timestamptz{Time: o.CreatedAt.UTC(), Valid: true}
	if o.CreatedAt.IsZero() {
		ts = pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}
	}

	res, err := queries.CreateOrder(ctx, sqlcgen.CreateOrderParams{
		ID:         o.ID,
		TenantID:   o.TenantID,
		UserID:     o.UserID,
		TotalCents: o.TotalCents,
		Status:     string(o.Status),
		CreatedAt:  ts,
		UpdatedAt:  ts,
	})
	if err != nil {
		return nil, err
	}

	return toDomainOrder(res, nil), nil
}

// CreateOrderItem inserts an order line item into the database using sqlc.
func (r *Repository) CreateOrderItem(ctx context.Context, item *OrderItem) (*OrderItem, error) {
	db := r.txManager.GetDB(ctx)
	queries := sqlcgen.New(db)

	ts := pgtype.Timestamptz{Time: item.CreatedAt.UTC(), Valid: true}
	if item.CreatedAt.IsZero() {
		ts = pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}
	}

	res, err := queries.CreateOrderItem(ctx, sqlcgen.CreateOrderItemParams{
		ID:             item.ID,
		OrderID:        item.OrderID,
		TenantID:       item.TenantID,
		ProductName:    item.ProductName,
		Quantity:       int32(item.Quantity),
		UnitPriceCents: item.UnitPriceCents,
		CreatedAt:      ts,
	})
	if err != nil {
		return nil, err
	}

	return toDomainOrderItem(res), nil
}

// GetByID retrieves an order and its line items by ID and tenant ID.
func (r *Repository) GetByID(ctx context.Context, tenantID, orderID string) (*Order, error) {
	db := r.txManager.GetDB(ctx)
	queries := sqlcgen.New(db)

	res, err := queries.GetOrderByID(ctx, sqlcgen.GetOrderByIDParams{
		ID:       orderID,
		TenantID: tenantID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrOrderNotFound
		}
		return nil, err
	}

	items, err := queries.ListOrderItems(ctx, sqlcgen.ListOrderItemsParams{
		OrderID:  orderID,
		TenantID: tenantID,
	})
	if err != nil {
		return nil, err
	}

	return toDomainOrder(res, items), nil
}

// ListOrderItems retrieves all line items for a specific order.
func (r *Repository) ListOrderItems(ctx context.Context, tenantID, orderID string) ([]*OrderItem, error) {
	db := r.txManager.GetDB(ctx)
	queries := sqlcgen.New(db)

	rows, err := queries.ListOrderItems(ctx, sqlcgen.ListOrderItemsParams{
		OrderID:  orderID,
		TenantID: tenantID,
	})
	if err != nil {
		return nil, err
	}

	items := make([]*OrderItem, len(rows))
	for i, row := range rows {
		items[i] = toDomainOrderItem(row)
	}
	return items, nil
}

// UpdateStatus updates the status of an order.
func (r *Repository) UpdateStatus(ctx context.Context, tenantID, orderID string, status OrderStatus) (*Order, error) {
	db := r.txManager.GetDB(ctx)
	queries := sqlcgen.New(db)

	ts := pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}
	res, err := queries.UpdateOrderStatus(ctx, sqlcgen.UpdateOrderStatusParams{
		ID:        orderID,
		TenantID:  tenantID,
		Status:    string(status),
		UpdatedAt: ts,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrOrderNotFound
		}
		return nil, err
	}

	return toDomainOrder(res, nil), nil
}

// Search executes dynamic multi-predicate queries using Bob SQL builder,
// preserving PostgreSQL index scans on (tenant_id, user_id) and (status).
func (r *Repository) Search(ctx context.Context, tenantID string, filter SearchFilter) ([]*Order, error) {
	mods := []bob.Mod[*dialect.SelectQuery]{
		sm.Columns("id", "tenant_id", "user_id", "total_cents", "status", "created_at", "updated_at", "deleted_at"),
		sm.From("orders"),
		sm.Where(psql.Quote("tenant_id").EQ(psql.Arg(tenantID))),
		sm.Where(psql.Quote("deleted_at").IsNull()),
	}

	if filter.Status != nil && *filter.Status != "" {
		mods = append(mods, sm.Where(psql.Quote("status").EQ(psql.Arg(string(*filter.Status)))))
	}
	if filter.UserID != nil && *filter.UserID != "" {
		mods = append(mods, sm.Where(psql.Quote("user_id").EQ(psql.Arg(*filter.UserID))))
	}
	if filter.MinAmountCents != nil {
		mods = append(mods, sm.Where(psql.Quote("total_cents").GTE(psql.Arg(*filter.MinAmountCents))))
	}
	if filter.MaxAmountCents != nil {
		mods = append(mods, sm.Where(psql.Quote("total_cents").LTE(psql.Arg(*filter.MaxAmountCents))))
	}

	mods = append(mods, sm.OrderBy("created_at").Desc())

	limit := int32(50)
	if filter.Limit > 0 {
		limit = filter.Limit
	}
	mods = append(mods, sm.Limit(limit))

	if filter.Offset > 0 {
		mods = append(mods, sm.Offset(filter.Offset))
	}

	q := psql.Select(mods...)
	sqlStr, args, err := bob.Build(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("failed to build dynamic order search query: %w", err)
	}

	db := r.txManager.GetDB(ctx)
	rows, err := db.Query(ctx, sqlStr, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to execute dynamic search query: %w", err)
	}
	defer rows.Close()

	var results []*Order
	for rows.Next() {
		var (
			id         string
			tID        string
			uID        string
			totalCents int64
			status     string
			createdAt  pgtype.Timestamptz
			updatedAt  pgtype.Timestamptz
			deletedAt  pgtype.Timestamptz
		)
		if err := rows.Scan(&id, &tID, &uID, &totalCents, &status, &createdAt, &updatedAt, &deletedAt); err != nil {
			return nil, fmt.Errorf("failed to scan order row: %w", err)
		}

		var delAt *time.Time
		if deletedAt.Valid {
			delAt = &deletedAt.Time
		}

		results = append(results, &Order{
			ID:         id,
			TenantID:   tID,
			UserID:     uID,
			TotalCents: totalCents,
			Status:     OrderStatus(status),
			CreatedAt:  createdAt.Time,
			UpdatedAt:  updatedAt.Time,
			DeletedAt:  delAt,
		})
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error reading order rows: %w", err)
	}

	return results, nil
}

func toDomainOrder(o sqlcgen.Orders, items []sqlcgen.OrderItems) *Order {
	var delAt *time.Time
	if o.DeletedAt.Valid {
		delAt = &o.DeletedAt.Time
	}

	var domainItems []*OrderItem
	if len(items) > 0 {
		domainItems = make([]*OrderItem, len(items))
		for i, it := range items {
			domainItems[i] = toDomainOrderItem(it)
		}
	}

	return &Order{
		ID:         o.ID,
		TenantID:   o.TenantID,
		UserID:     o.UserID,
		TotalCents: o.TotalCents,
		Status:     OrderStatus(o.Status),
		Items:      domainItems,
		CreatedAt:  o.CreatedAt.Time,
		UpdatedAt:  o.UpdatedAt.Time,
		DeletedAt:  delAt,
	}
}

func toDomainOrderItem(it sqlcgen.OrderItems) *OrderItem {
	return &OrderItem{
		ID:             it.ID,
		OrderID:        it.OrderID,
		TenantID:       it.TenantID,
		ProductName:    it.ProductName,
		Quantity:       int(it.Quantity),
		UnitPriceCents: it.UnitPriceCents,
		CreatedAt:      it.CreatedAt.Time,
	}
}
