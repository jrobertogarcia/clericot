package config

import (
	"fmt"
	"time"

	"github.com/caarlos0/env/v11"
)

// Config represents the application and platform configuration parsed from environment variables.
type Config struct {
	App      AppConfig      `envPrefix:"APP_"`
	Database DatabaseConfig `envPrefix:"DB_"`
	Redis    RedisConfig    `envPrefix:"REDIS_"`
	Storage  StorageConfig  `envPrefix:"STORAGE_"`
	Auth     AuthConfig     `envPrefix:"AUTH_"`
	Events   EventsConfig   `envPrefix:"EVENTS_"`
	OTel     OTelConfig     `envPrefix:"OTEL_"`
}

type AppConfig struct {
	Env      string `env:"ENV" envDefault:"development"`
	Port     int    `env:"PORT" envDefault:"8080"`
	Name     string `env:"NAME" envDefault:"clericot"`
	LogLevel string `env:"LOG_LEVEL" envDefault:"info"`
}

type DatabaseConfig struct {
	URL             string        `env:"DATABASE_URL" envDefault:"postgres://postgres:postgrespassword@localhost:5432/clericot?sslmode=disable"`
	AdminURL        string        `env:"ADMIN_DATABASE_URL" envDefault:"postgres://postgres:postgrespassword@localhost:5432/clericot?sslmode=disable"`
	MaxConns        int32         `env:"MAX_CONNS" envDefault:"25"`
	MinConns        int32         `env:"MIN_CONNS" envDefault:"5"`
	MaxConnLifetime time.Duration `env:"MAX_CONN_LIFETIME" envDefault:"30m"`
	MaxConnIdleTime time.Duration `env:"MAX_CONN_IDLE_TIME" envDefault:"5m"`
}

type RedisConfig struct {
	URL string `env:"URL" envDefault:"redis://localhost:6379/0"`
}

type StorageConfig struct {
	BucketURL string `env:"BUCKET_URL" envDefault:"file:///tmp/clericot-storage"`
}

type EventsConfig struct {
	Driver string `env:"DRIVER" envDefault:"redis"`
}

type AuthConfig struct {
	JWTSecret       string `env:"JWT_SECRET" envDefault:"dev-super-secret-jwt-signing-key-32b"`
	SessionSecret   string `env:"SESSION_SECRET" envDefault:"dev-super-secret-session-key-32b"`
	KMSKeyURI       string `env:"KMS_KEY_URI" envDefault:""`
	Argon2MemoryMB  uint32 `env:"ARGON2_MEMORY_MB" envDefault:"64"`
	Argon2Time      uint32 `env:"ARGON2_TIME" envDefault:"3"`
	Argon2Threads   uint8  `env:"ARGON2_THREADS" envDefault:"4"`
}

type OTelConfig struct {
	Endpoint      string  `env:"EXPORTER_OTLP_ENDPOINT" envDefault:""`
	SamplingRatio float64 `env:"SAMPLING_RATIO" envDefault:"0.05"`
}

// Load parses environment variables into a typed Config struct.
func Load() (*Config, error) {
	cfg := &Config{}
	if err := env.Parse(cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config from environment: %w", err)
	}
	return cfg, nil
}
