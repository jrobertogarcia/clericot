package auth

import (
	"time"

	"github.com/alexedwards/scs/goredisstore"
	"github.com/alexedwards/scs/v2"
	"github.com/redis/go-redis/v9"
)

// NewSessionManager configures a production-ready SCS session manager backed by Redis.
func NewSessionManager(rdb *redis.Client, lifetime time.Duration) *scs.SessionManager {
	sessionManager := scs.New()
	sessionManager.Lifetime = lifetime
	if lifetime == 0 {
		sessionManager.Lifetime = 24 * time.Hour
	}
	sessionManager.Cookie.Name = "clericot_session"
	sessionManager.Cookie.HttpOnly = true
	sessionManager.Cookie.SameSite = 3 // http.SameSiteStrictMode
	sessionManager.Cookie.Secure = true

	if rdb != nil {
		sessionManager.Store = goredisstore.New(rdb)
	}

	return sessionManager
}
