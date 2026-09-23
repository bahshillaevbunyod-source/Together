// Package schema provides a lightweight compatibility check between the running
// backend and the live PostgreSQL schema. It exists to catch schema drift
// (unapplied migrations) at startup and readiness time, instead of letting every
// request fail with an opaque 500. It performs a small, fixed set of catalog
// lookups — never a per-request or per-column scan of application queries.
package schema

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// Queryer is the minimal read-only executor this package needs. *pgxpool.Pool
// and pgx.Conn both satisfy it.
type Queryer interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// requiredColumns are columns the application's core queries depend on that were
// added by later migrations (the ones whose absence caused the auth outage).
var requiredColumns = []struct{ table, column string }{
	{"users", "platform_language"},      // 000017
	{"users", "preferred_language"},     // 000015
	{"users", "auto_translate_enabled"}, // 000015
	{"users", "is_private"},             // 000018
	{"messages", "source_language"},     // 000024
	{"messages", "source_language_confidence"},
	{"messages", "source_language_resolution"},
}

// requiredTables are tables the application depends on from later migrations.
var requiredTables = []string{
	"follow_requests", // 000019
	"stories",         // 000021
	"story_views",     // 000022
}

// Verify returns the list of missing required schema objects (human-readable
// names only, never data). An empty slice means the schema is compatible. A
// non-nil error means the check itself could not run (e.g. DB unreachable).
func Verify(ctx context.Context, q Queryer) ([]string, error) {
	var missing []string

	for _, rc := range requiredColumns {
		var exists bool
		if err := q.QueryRow(ctx,
			`SELECT EXISTS (SELECT 1 FROM information_schema.columns
			 WHERE table_schema='public' AND table_name=$1 AND column_name=$2)`,
			rc.table, rc.column,
		).Scan(&exists); err != nil {
			return nil, fmt.Errorf("schema check (column %s.%s): %w", rc.table, rc.column, err)
		}
		if !exists {
			missing = append(missing, fmt.Sprintf("column %s.%s", rc.table, rc.column))
		}
	}

	for _, t := range requiredTables {
		var exists bool
		if err := q.QueryRow(ctx,
			`SELECT EXISTS (SELECT 1 FROM information_schema.tables
			 WHERE table_schema='public' AND table_name=$1)`, t,
		).Scan(&exists); err != nil {
			return nil, fmt.Errorf("schema check (table %s): %w", t, err)
		}
		if !exists {
			missing = append(missing, fmt.Sprintf("table %s", t))
		}
	}

	return missing, nil
}
