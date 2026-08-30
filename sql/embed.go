package sql

import "embed"

// MigrationsFS holds all embedded SQL migration scripts.
//
//go:embed migrations/*.sql
var MigrationsFS embed.FS
