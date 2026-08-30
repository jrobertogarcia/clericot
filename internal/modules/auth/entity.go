package auth

import (
	"errors"
	"time"
)

// Role defines the authorization level for a user within a tenant.
type Role string

const (
	RoleAdmin  Role = "admin"
	RoleMember Role = "member"
)

// User represents a pure domain user entity decoupled from persistence models.
type User struct {
	ID           string
	TenantID     string
	Email        string
	Name         string
	PasswordHash string
	Role         Role
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// Tenant represents a tenant organizational domain entity.
type Tenant struct {
	ID        string
	Name      string
	Status    string
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt *time.Time
}

// RegisterDTO contains input data required to register a new user.
type RegisterDTO struct {
	TenantID string
	Email    string
	Password string
	Name     string
}

// LoginDTO contains input data required to authenticate a user.
type LoginDTO struct {
	TenantID string
	Email    string
	Password string
}

// AuthToken represents an issued authentication security token.
type AuthToken struct {
	AccessToken string
	ExpiresAt   time.Time
	TokenType   string
}

// Domain error sentinels.
var (
	ErrUserNotFound       = errors.New("user not found")
	ErrUserAlreadyExists   = errors.New("user with this email already exists")
	ErrInvalidCredentials = errors.New("invalid email or password")
	ErrTenantNotFound     = errors.New("tenant not found")
	ErrTenantRequired     = errors.New("tenant_id is required")
	ErrUnauthenticated    = errors.New("unauthenticated request")
)
