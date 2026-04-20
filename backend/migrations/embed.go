package migrations

import "embed"

// Files contains all SQL migration files in this directory.
//go:embed *.sql
var Files embed.FS
