package auth

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	platformAuth "clericot/internal/platform/auth"
	"clericot/internal/platform/database"
	"clericot/internal/platform/security"
	"clericot/internal/platform/tenant"
)

// Service encapsulates business logic for authentication and user identities.
type Service struct {
	repo         *Repository
	txManager    *database.TxManager
	tokenService *platformAuth.TokenService
}

// NewService constructs a new auth Service instance.
func NewService(repo *Repository, txManager *database.TxManager, tokenService *platformAuth.TokenService) *Service {
	return &Service{
		repo:         repo,
		txManager:    txManager,
		tokenService: tokenService,
	}
}

// Register registers a new user within a tenant and issues an authentication token.
func (s *Service) Register(ctx context.Context, dto RegisterDTO) (*AuthToken, *User, error) {
	if dto.TenantID == "" {
		return nil, nil, ErrTenantRequired
	}

	hashedPassword, err := security.HashPassword(dto.Password, security.DefaultArgon2Params())
	if err != nil {
		return nil, nil, err
	}

	userID := uuid.NewString()
	now := time.Now().UTC()

	var createdUser *User
	err = s.txManager.RunInTx(tenant.WithTenant(ctx, dto.TenantID), func(txCtx context.Context) error {
		existing, err := s.repo.GetUserByEmail(txCtx, dto.TenantID, dto.Email)
		if err == nil && existing != nil {
			return ErrUserAlreadyExists
		} else if err != nil && !errors.Is(err, ErrUserNotFound) {
			return err
		}

		u, err := s.repo.CreateUser(txCtx, &User{
			ID:           userID,
			TenantID:     dto.TenantID,
			Email:        dto.Email,
			Name:         dto.Name,
			PasswordHash: hashedPassword,
			Role:         RoleMember,
			CreatedAt:    now,
			UpdatedAt:    now,
		})
		if err != nil {
			return err
		}
		createdUser = u
		return nil
	})
	if err != nil {
		return nil, nil, err
	}

	principal := &platformAuth.AuthPrincipal{
		ID:       createdUser.ID,
		TenantID: createdUser.TenantID,
		Email:    createdUser.Email,
		Role:     string(createdUser.Role),
		Tenants:  []string{createdUser.TenantID},
	}

	expiry := 24 * time.Hour
	token, err := s.tokenService.GenerateToken(principal, expiry)
	if err != nil {
		return nil, nil, err
	}

	authToken := &AuthToken{
		AccessToken: token,
		ExpiresAt:   time.Now().Add(expiry),
		TokenType:   "Bearer",
	}

	return authToken, createdUser, nil
}

// Login verifies credentials and issues a security token.
func (s *Service) Login(ctx context.Context, dto LoginDTO) (*AuthToken, *User, error) {
	if dto.TenantID == "" {
		return nil, nil, ErrTenantRequired
	}

	var user *User
	err := s.txManager.RunInTx(tenant.WithTenant(ctx, dto.TenantID), func(txCtx context.Context) error {
		u, err := s.repo.GetUserByEmail(txCtx, dto.TenantID, dto.Email)
		if err != nil {
			if errors.Is(err, ErrUserNotFound) {
				return ErrInvalidCredentials
			}
			return err
		}
		user = u
		return nil
	})
	if err != nil {
		return nil, nil, err
	}

	match, err := security.VerifyPassword(dto.Password, user.PasswordHash)
	if err != nil || !match {
		return nil, nil, ErrInvalidCredentials
	}

	principal := &platformAuth.AuthPrincipal{
		ID:       user.ID,
		TenantID: user.TenantID,
		Email:    user.Email,
		Role:     string(user.Role),
		Tenants:  []string{user.TenantID},
	}

	expiry := 24 * time.Hour
	token, err := s.tokenService.GenerateToken(principal, expiry)
	if err != nil {
		return nil, nil, err
	}

	authToken := &AuthToken{
		AccessToken: token,
		ExpiresAt:   time.Now().Add(expiry),
		TokenType:   "Bearer",
	}

	return authToken, user, nil
}

// GetMe retrieves the profile of the currently authenticated principal.
func (s *Service) GetMe(ctx context.Context) (*User, error) {
	principal := platformAuth.PrincipalFromContext(ctx)
	if principal == nil {
		return nil, ErrUnauthenticated
	}

	var user *User
	err := s.txManager.RunInTx(tenant.WithTenant(ctx, principal.TenantID), func(txCtx context.Context) error {
		u, err := s.repo.GetUserByID(txCtx, principal.TenantID, principal.ID)
		if err != nil {
			return err
		}
		user = u
		return nil
	})
	if err != nil {
		return nil, err
	}

	return user, nil
}
