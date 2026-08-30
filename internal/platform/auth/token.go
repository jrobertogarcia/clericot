package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/lestrrat-go/jwx/v2/jwa"
	"github.com/lestrrat-go/jwx/v2/jwt"
	"github.com/redis/go-redis/v9"
)

var (
	ErrInvalidToken = errors.New("invalid or expired authentication token")
	ErrTokenRevoked = errors.New("token has been revoked")
)

type TokenService struct {
	secretKey []byte
	redis     *redis.Client
}

func NewTokenService(secret string, rdb *redis.Client) *TokenService {
	return &TokenService{
		secretKey: []byte(secret),
		redis:     rdb,
	}
}

// GenerateToken issues a signed JWT token with a unique JTI identifier.
func (s *TokenService) GenerateToken(principal *AuthPrincipal, ttl time.Duration) (string, error) {
	now := time.Now().UTC()
	jti := uuid.NewString()

	builder := jwt.NewBuilder().
		JwtID(jti).
		Subject(principal.ID).
		IssuedAt(now).
		Expiration(now.Add(ttl)).
		Claim("tenant_id", principal.TenantID).
		Claim("email", principal.Email).
		Claim("role", principal.Role)

	if len(principal.Tenants) > 0 {
		builder.Claim("tenants", principal.Tenants)
	}

	tok, err := builder.Build()
	if err != nil {
		return "", fmt.Errorf("failed to build jwt token: %w", err)
	}

	signed, err := jwt.Sign(tok, jwt.WithKey(jwa.HS256, s.secretKey))
	if err != nil {
		return "", fmt.Errorf("failed to sign jwt token: %w", err)
	}

	return string(signed), nil
}

// ValidateToken parses, validates signature, checks expiration, and checks Redis JTI blocklist.
func (s *TokenService) ValidateToken(ctx context.Context, tokenString string) (*AuthPrincipal, error) {
	tok, err := jwt.Parse([]byte(tokenString), jwt.WithKey(jwa.HS256, s.secretKey), jwt.WithValidate(true))
	if err != nil {
		return nil, ErrInvalidToken
	}

	jti := tok.JwtID()
	if jti != "" && s.redis != nil {
		revoked, err := s.IsTokenRevoked(ctx, jti)
		if err == nil && revoked {
			return nil, ErrTokenRevoked
		}
	}

	tenantID, _ := tok.Get("tenant_id")
	email, _ := tok.Get("email")
	role, _ := tok.Get("role")

	principal := &AuthPrincipal{
		ID:       tok.Subject(),
		TenantID: fmt.Sprint(tenantID),
		Email:    fmt.Sprint(email),
		Role:     fmt.Sprint(role),
	}

	if rawTenants, ok := tok.Get("tenants"); ok {
		if tenantsSlice, ok := rawTenants.([]any); ok {
			for _, t := range tenantsSlice {
				principal.Tenants = append(principal.Tenants, fmt.Sprint(t))
			}
		}
	}

	return principal, nil
}

// RevokeToken adds the token JTI to the Redis revocation blocklist.
func (s *TokenService) RevokeToken(ctx context.Context, jti string, ttl time.Duration) error {
	if s.redis == nil {
		return nil
	}
	key := fmt.Sprintf("auth:revoked:%s", jti)
	return s.redis.Set(ctx, key, "1", ttl).Err()
}

// IsTokenRevoked checks if a JTI exists in the Redis revocation blocklist.
func (s *TokenService) IsTokenRevoked(ctx context.Context, jti string) (bool, error) {
	if s.redis == nil {
		return false, nil
	}
	key := fmt.Sprintf("auth:revoked:%s", jti)
	exists, err := s.redis.Exists(ctx, key).Result()
	if err != nil {
		return false, err
	}
	return exists > 0, nil
}
