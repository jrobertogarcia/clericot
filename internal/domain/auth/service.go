package auth

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	platformAuth "clericot/internal/platform/auth"
	"clericot/internal/platform/database"
	"clericot/internal/platform/httperr"
	"clericot/internal/platform/security"
	"clericot/internal/platform/tenant"
	"clericot/internal/sqlcgen"
)

type AuthService struct {
	txManager    *database.TxManager
	tokenService *platformAuth.TokenService
}

func NewAuthService(txManager *database.TxManager, tokenService *platformAuth.TokenService) *AuthService {
	return &AuthService{
		txManager:    txManager,
		tokenService: tokenService,
	}
}

func (s *AuthService) Register(ctx context.Context, tenantID, email, password, name string) (string, time.Time, error) {
	if tenantID == "" {
		return "", time.Time{}, httperr.NewBadRequest("tenant_id is required")
	}

	hashedPassword, err := security.HashPassword(password, security.DefaultArgon2Params())
	if err != nil {
		return "", time.Time{}, httperr.NewInternal("failed to process password")
	}

	userID := uuid.NewString()
	ts := pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}

	var user sqlcgen.Users
	err = s.txManager.RunInTx(tenant.WithTenant(ctx, tenantID), func(txCtx context.Context) error {
		db := s.txManager.GetDB(txCtx)
		queries := sqlcgen.New(db)

		// Check if user already exists
		existing, _ := queries.GetUserByEmail(txCtx, sqlcgen.GetUserByEmailParams{
			Email:    email,
			TenantID: tenantID,
		})
		if existing.ID != "" {
			return httperr.NewConflict("user with this email already exists")
		}

		u, err := queries.CreateUser(txCtx, sqlcgen.CreateUserParams{
			ID:           userID,
			TenantID:     tenantID,
			Email:        email,
			PasswordHash: hashedPassword,
			Name:         name,
			Role:         "member",
			CreatedAt:    ts,
			UpdatedAt:    ts,
		})
		if err != nil {
			return err
		}
		user = u
		return nil
	})
	if err != nil {
		return "", time.Time{}, httperr.Transform(err)
	}

	principal := &platformAuth.AuthPrincipal{
		ID:       user.ID,
		TenantID: user.TenantID,
		Email:    user.Email,
		Role:     user.Role,
		Tenants:  []string{user.TenantID},
	}

	expiry := 24 * time.Hour
	token, err := s.tokenService.GenerateToken(principal, expiry)
	if err != nil {
		return "", time.Time{}, httperr.NewInternal("failed to issue security token")
	}

	return token, time.Now().Add(expiry), nil
}

func (s *AuthService) Login(ctx context.Context, tenantID, email, password string) (string, time.Time, error) {
	var user sqlcgen.Users
	err := s.txManager.RunInTx(tenant.WithTenant(ctx, tenantID), func(txCtx context.Context) error {
		db := s.txManager.GetDB(txCtx)
		queries := sqlcgen.New(db)

		u, err := queries.GetUserByEmail(txCtx, sqlcgen.GetUserByEmailParams{
			Email:    email,
			TenantID: tenantID,
		})
		if err != nil {
			return httperr.NewUnauthorized("invalid email or password")
		}
		user = u
		return nil
	})
	if err != nil {
		return "", time.Time{}, httperr.Transform(err)
	}

	match, err := security.VerifyPassword(password, user.PasswordHash)
	if err != nil || !match {
		return "", time.Time{}, httperr.NewUnauthorized("invalid email or password")
	}

	principal := &platformAuth.AuthPrincipal{
		ID:       user.ID,
		TenantID: user.TenantID,
		Email:    user.Email,
		Role:     user.Role,
		Tenants:  []string{user.TenantID},
	}

	expiry := 24 * time.Hour
	token, err := s.tokenService.GenerateToken(principal, expiry)
	if err != nil {
		return "", time.Time{}, httperr.NewInternal("failed to issue security token")
	}

	return token, time.Now().Add(expiry), nil
}

func (s *AuthService) GetMe(ctx context.Context) (*sqlcgen.Users, error) {
	principal := platformAuth.PrincipalFromContext(ctx)
	if principal == nil {
		return nil, httperr.NewUnauthorized("unauthenticated request")
	}

	var user sqlcgen.Users
	err := s.txManager.RunInTx(tenant.WithTenant(ctx, principal.TenantID), func(txCtx context.Context) error {
		db := s.txManager.GetDB(txCtx)
		queries := sqlcgen.New(db)

		u, err := queries.GetUserByID(txCtx, sqlcgen.GetUserByIDParams{
			ID:       principal.ID,
			TenantID: principal.TenantID,
		})
		if err != nil {
			return httperr.NewNotFound("user profile not found")
		}
		user = u
		return nil
	})
	if err != nil {
		return nil, httperr.Transform(err)
	}

	return &user, nil
}
