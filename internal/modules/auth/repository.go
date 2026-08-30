package auth

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"clericot/internal/platform/database"
	"clericot/internal/sqlcgen"
)

// Repository handles database operations for the auth domain.
type Repository struct {
	txManager *database.TxManager
}

// NewRepository creates a new auth Repository instance.
func NewRepository(txManager *database.TxManager) *Repository {
	return &Repository{
		txManager: txManager,
	}
}

// CreateUser inserts a new user record into the database and returns the domain entity.
func (r *Repository) CreateUser(ctx context.Context, u *User) (*User, error) {
	db := r.txManager.GetDB(ctx)
	queries := sqlcgen.New(db)

	ts := pgtype.Timestamptz{Time: u.CreatedAt.UTC(), Valid: true}
	if u.CreatedAt.IsZero() {
		ts = pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}
	}

	res, err := queries.CreateUser(ctx, sqlcgen.CreateUserParams{
		ID:           u.ID,
		TenantID:     u.TenantID,
		Email:        u.Email,
		Name:         u.Name,
		PasswordHash: u.PasswordHash,
		Role:         string(u.Role),
		CreatedAt:    ts,
		UpdatedAt:    ts,
	})
	if err != nil {
		return nil, err
	}

	return toDomainUser(res), nil
}

// GetUserByEmail retrieves a user by tenant ID and email.
func (r *Repository) GetUserByEmail(ctx context.Context, tenantID, email string) (*User, error) {
	db := r.txManager.GetDB(ctx)
	queries := sqlcgen.New(db)

	res, err := queries.GetUserByEmail(ctx, sqlcgen.GetUserByEmailParams{
		Email:    email,
		TenantID: tenantID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}

	return toDomainUser(res), nil
}

// GetUserByID retrieves a user by tenant ID and user ID.
func (r *Repository) GetUserByID(ctx context.Context, tenantID, id string) (*User, error) {
	db := r.txManager.GetDB(ctx)
	queries := sqlcgen.New(db)

	res, err := queries.GetUserByID(ctx, sqlcgen.GetUserByIDParams{
		ID:       id,
		TenantID: tenantID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}

	return toDomainUser(res), nil
}

// ListUsersByTenant returns paginated users for a tenant.
func (r *Repository) ListUsersByTenant(ctx context.Context, tenantID string, limit, offset int32) ([]*User, error) {
	db := r.txManager.GetDB(ctx)
	queries := sqlcgen.New(db)

	rows, err := queries.ListUsersByTenant(ctx, sqlcgen.ListUsersByTenantParams{
		TenantID: tenantID,
		Limit:    limit,
		Offset:   offset,
	})
	if err != nil {
		return nil, err
	}

	users := make([]*User, len(rows))
	for i, row := range rows {
		users[i] = toDomainUser(row)
	}
	return users, nil
}

// DeleteUser deletes a user by ID and tenant ID.
func (r *Repository) DeleteUser(ctx context.Context, tenantID, id string) error {
	db := r.txManager.GetDB(ctx)
	queries := sqlcgen.New(db)

	return queries.DeleteUser(ctx, sqlcgen.DeleteUserParams{
		ID:       id,
		TenantID: tenantID,
	})
}

func toDomainUser(u sqlcgen.Users) *User {
	return &User{
		ID:           u.ID,
		TenantID:     u.TenantID,
		Email:        u.Email,
		Name:         u.Name,
		PasswordHash: u.PasswordHash,
		Role:         Role(u.Role),
		CreatedAt:    u.CreatedAt.Time,
		UpdatedAt:    u.UpdatedAt.Time,
	}
}
