package config_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"clericot/internal/config"
)

func TestLoad_Defaults(t *testing.T) {
	cfg, err := config.Load()
	require.NoError(t, err)
	require.NotNil(t, cfg)

	assert.Equal(t, "development", cfg.App.Env)
	assert.Equal(t, 8080, cfg.App.Port)
	assert.Equal(t, "clericot", cfg.App.Name)
	assert.Equal(t, int32(25), cfg.Database.MaxConns)
	assert.Equal(t, 30*time.Minute, cfg.Database.MaxConnLifetime)
	assert.Equal(t, "redis://localhost:6379/0", cfg.Redis.URL)
	assert.Equal(t, uint32(64), cfg.Auth.Argon2MemoryMB)
	assert.Equal(t, 0.05, cfg.OTel.SamplingRatio)
}

func TestLoad_CustomEnv(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("APP_PORT", "9090")
	t.Setenv("DB_MAX_CONNS", "50")

	cfg, err := config.Load()
	require.NoError(t, err)
	require.NotNil(t, cfg)

	assert.Equal(t, "production", cfg.App.Env)
	assert.Equal(t, 9090, cfg.App.Port)
	assert.Equal(t, int32(50), cfg.Database.MaxConns)
}
