package auth

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
)

// RegisterRoutes registers Huma v2 typed OpenAPI operations for authentication.
func RegisterRoutes(api huma.API, svc *AuthService) {
	huma.Register(api, huma.Operation{
		OperationID: "register-user",
		Method:      http.MethodPost,
		Path:        "/v1/auth/register",
		Summary:     "Register a new user",
		Tags:        []string{"Authentication"},
	}, func(ctx context.Context, input *RegisterInput) (*AuthTokenResponse, error) {
		token, expiresAt, err := svc.Register(ctx, input.Body.TenantID, input.Body.Email, input.Body.Password, input.Body.Name)
		if err != nil {
			return nil, err
		}

		resp := &AuthTokenResponse{}
		resp.Body.AccessToken = token
		resp.Body.ExpiresAt = expiresAt
		resp.Body.TokenType = "Bearer"
		return resp, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "login-user",
		Method:      http.MethodPost,
		Path:        "/v1/auth/login",
		Summary:     "Authenticate user credentials",
		Tags:        []string{"Authentication"},
	}, func(ctx context.Context, input *LoginInput) (*AuthTokenResponse, error) {
		token, expiresAt, err := svc.Login(ctx, input.Body.TenantID, input.Body.Email, input.Body.Password)
		if err != nil {
			return nil, err
		}

		resp := &AuthTokenResponse{}
		resp.Body.AccessToken = token
		resp.Body.ExpiresAt = expiresAt
		resp.Body.TokenType = "Bearer"
		return resp, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-current-user",
		Method:      http.MethodGet,
		Path:        "/v1/auth/me",
		Summary:     "Get currently authenticated user profile",
		Tags:        []string{"Authentication"},
	}, func(ctx context.Context, input *struct{}) (*UserProfileResponse, error) {
		user, err := svc.GetMe(ctx)
		if err != nil {
			return nil, err
		}

		resp := &UserProfileResponse{}
		resp.Body.ID = user.ID
		resp.Body.TenantID = user.TenantID
		resp.Body.Email = user.Email
		resp.Body.Name = user.Name
		resp.Body.Role = user.Role
		resp.Body.CreatedAt = user.CreatedAt.Time
		return resp, nil
	})
}
