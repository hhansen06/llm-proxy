package store

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"sort"
	"strings"

	"llm-proxy/backend/migrations"
)

func ApplyMigrations(ctx context.Context, db *sql.DB) error {
	files, err := fs.Glob(migrations.Files, "*.sql")
	if err != nil {
		return fmt.Errorf("glob migration files: %w", err)
	}
	sort.Strings(files)

	for _, name := range files {
		b, err := migrations.Files.ReadFile(name)
		if err != nil {
			return fmt.Errorf("read migration file %s: %w", name, err)
		}

		statements := splitSQLStatements(string(b))
		for i, stmt := range statements {
			if _, err := db.ExecContext(ctx, stmt); err != nil {
				return fmt.Errorf("apply migration %s statement %d: %w", name, i+1, err)
			}
		}
	}

	return nil
}

func splitSQLStatements(sqlText string) []string {
	lines := strings.Split(sqlText, "\n")
	filtered := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "--") {
			continue
		}
		filtered = append(filtered, line)
	}

	joined := strings.Join(filtered, "\n")
	parts := strings.Split(joined, ";")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		stmt := strings.TrimSpace(part)
		if stmt == "" {
			continue
		}
		out = append(out, stmt)
	}
	return out
}
