package auth

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"clericot/internal/platform/httperr"
)

// RegisterInput represents HTTP request parameters for user registration.
type RegisterInput struct {
	Body struct {
		TenantID string `json:"tenant_id" doc:"Tenant identifier" example:"tenant_123"`
		Email    string `json:"email" doc:"User email address" format:"email" example:"user@example.com"`
		Password string `json:"password" doc:"User plaintext password" minLength:"8" example:"SuperSecret2026!"`
		Name     string `json:"name" doc:"User full name" minLength:"2" example:"Jane Doe"`
	}
}

// LoginInput represents HTTP request parameters for user authentication.
type LoginInput struct {
	Body struct {
		TenantID string `json:"tenant_id" doc:"Tenant identifier" example:"tenant_123"`
		Email    string `json:"email" doc:"User email address" format:"email" example:"user@example.com"`
		Password string `json:"password" doc:"User plaintext password" example:"SuperSecret2026!"`
	}
}

// AuthTokenResponse represents the issued JWT token response payload.
type AuthTokenResponse struct {
	Body struct {
		AccessToken string    `json:"access_token" doc:"Bearer JWT security token"`
		ExpiresAt   time.Time `json:"expires_at" doc:"Token expiration timestamp"`
		TokenType   string    `json:"token_type" doc:"Token type" default:"Bearer"`
	}
}

// UserProfileResponse represents the authenticated user profile payload.
type UserProfileResponse struct {
	Body struct {
		ID        string    `json:"id" doc:"Unique user ID"`
		TenantID  string    `json:"tenant_id" doc:"Tenant ID"`
		Email     string    `json:"email" doc:"User email address"`
		Name      string    `json:"name" doc:"User full name"`
		Role      string    `json:"role" doc:"User role"`
		CreatedAt time.Time `json:"created_at" doc:"Creation timestamp"`
	}
}

// Handler handles HTTP transport operations for the auth domain.
type Handler struct {
	svc *Service
}

// NewHandler creates a new auth Handler.
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// RegisterRoutes registers Huma v2 typed OpenAPI operations for authentication.
func (h *Handler) RegisterRoutes(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "register-user",
		Method:      http.MethodPost,
		Path:        "/v1/auth/register",
		Summary:     "Register a new user",
		Description: "Creates a new user record within the specified tenant and returns an authentication token.",
		Tags:        []string{"Authentication"},
	}, func(ctx context.Context, input *RegisterInput) (*AuthTokenResponse, error) {
		token, _, err := h.svc.Register(ctx, RegisterDTO{
			TenantID: input.Body.TenantID,
			Email:    input.Body.Email,
			Password: input.Body.Password,
			Name:     input.Body.Name,
		})
		if err != nil {
			return nil, mapDomainError(err)
		}

		resp := &AuthTokenResponse{}
		resp.Body.AccessToken = token.AccessToken
		resp.Body.ExpiresAt = token.ExpiresAt
		resp.Body.TokenType = token.TokenType
		return resp, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "login-user",
		Method:      http.MethodPost,
		Path:        "/v1/auth/login",
		Summary:     "Authenticate user credentials",
		Description: "Validates user credentials within a tenant and issues a Bearer JWT token.",
		Tags:        []string{"Authentication"},
	}, func(ctx context.Context, input *LoginInput) (*AuthTokenResponse, error) {
		token, _, err := h.svc.Login(ctx, LoginDTO{
			TenantID: input.Body.TenantID,
			Email:    input.Body.Email,
			Password: input.Body.Password,
		})
		if err != nil {
			return nil, mapDomainError(err)
		}

		resp := &AuthTokenResponse{}
		resp.Body.AccessToken = token.AccessToken
		resp.Body.ExpiresAt = token.ExpiresAt
		resp.Body.TokenType = token.TokenType
		return resp, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-current-user",
		Method:      http.MethodGet,
		Path:        "/v1/auth/me",
		Summary:     "Get currently authenticated user profile",
		Description: "Retrieves profile data for the authenticated user principal from the request context.",
		Tags:        []string{"Authentication"},
	}, func(ctx context.Context, input *struct{}) (*UserProfileResponse, error) {
		user, err := h.svc.GetMe(ctx)
		if err != nil {
			return nil, mapDomainError(err)
		}

		resp := &UserProfileResponse{}
		resp.Body.ID = user.ID
		resp.Body.TenantID = user.TenantID
		resp.Body.Email = user.Email
		resp.Body.Name = user.Name
		resp.Body.Role = string(user.Role)
		resp.Body.CreatedAt = user.CreatedAt
		return resp, nil
	})
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
	case errors.Is(err, ErrUserAlreadyExists):
		return httperr.NewConflict(err.Error())
	case errors.Is(err, ErrInvalidCredentials):
		return httperr.NewUnauthorized(err.Error())
	case errors.Is(err, ErrUserNotFound), errors.Is(err, ErrTenantNotFound):
		return httperr.NewNotFound(err.Error())
	case errors.Is(err, ErrTenantRequired):
		return httperr.NewBadRequest(err.Error())
	case errors.Is(err, ErrUnauthenticated):
		return httperr.NewUnauthorized(err.Error())
	default:
		return httperr.Transform(err)
	}
}
