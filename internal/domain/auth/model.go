package auth

import "time"

type RegisterInput struct {
	Body struct {
		TenantID string `json:"tenant_id" doc:"Tenant identifier"`
		Email    string `json:"email" doc:"User email address" format:"email"`
		Password string `json:"password" doc:"User plaintext password" minLength:"8"`
		Name     string `json:"name" doc:"User full name" minLength:"2"`
	}
}

type LoginInput struct {
	Body struct {
		TenantID string `json:"tenant_id" doc:"Tenant identifier"`
		Email    string `json:"email" doc:"User email address" format:"email"`
		Password string `json:"password" doc:"User plaintext password"`
	}
}

type AuthTokenResponse struct {
	Body struct {
		AccessToken string    `json:"access_token" doc:"Bearer JWT token"`
		ExpiresAt   time.Time `json:"expires_at" doc:"Token expiration timestamp"`
		TokenType   string    `json:"token_type" doc:"Token type" default:"Bearer"`
	}
}

type UserProfileResponse struct {
	Body struct {
		ID        string    `json:"id"`
		TenantID  string    `json:"tenant_id"`
		Email     string    `json:"email"`
		Name      string    `json:"name"`
		Role      string    `json:"role"`
		CreatedAt time.Time `json:"created_at"`
	}
}
