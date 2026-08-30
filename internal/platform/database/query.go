package database

import (
	"context"

	"github.com/stephenafamo/bob"
	"github.com/stephenafamo/bob/dialect/psql"
)

// Expression is an alias for PostgreSQL query expressions in Bob.
type Expression = psql.Expression

// BuildSQL converts a Bob Query into a parameterized SQL statement with arguments.
// This enables dynamic multi-predicate queries that preserve PostgreSQL index scans (Rule 6).
func BuildSQL(ctx context.Context, q bob.Query) (string, []any, error) {
	return bob.Build(ctx, q)
}
