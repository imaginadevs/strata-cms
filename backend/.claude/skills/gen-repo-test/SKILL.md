---
name: gen-repo-test
description: Generate table-driven Go tests for a Postgres repository in internal/adapters/persistence/postgres, focusing on tenant-schema isolation. Use when adding test coverage for repository or DDL code.
disable-model-invocation: true
---

# gen-repo-test

Generate `*_test.go` coverage for a repository in
`internal/adapters/persistence/postgres/`. The project has no tests yet, so establish
a clean, idiomatic pattern others can copy.

## Approach

1. **Read the target repository file first** and list its exported methods, their
   inputs, and the tenant/schema they operate on. Tests must assert real behavior —
   never invent method signatures.
2. Prefer a **real Postgres** via a build tag so unit runs stay fast:
   - Guard DB tests with `//go:build integration` at the top of the file.
   - Connect using env (`TEST_DATABASE_URL`, fall back to the dev DSN from config).
     `stdlibDB` already wraps the pool for packages needing `*sql.DB`.
   - In setup, create a throwaway tenant schema; in teardown, `DROP SCHEMA ... CASCADE`.
     Use `t.Cleanup` so failures still clean up.
3. Use **table-driven** subtests (`t.Run`) with a `name`, inputs, and want/expected.
4. **Always include a tenant-isolation case**: create data under tenant A, then assert
   a read scoped to tenant B does NOT see it. This is the highest-value assertion in
   this codebase.
5. Cover: happy path, not-found, validation rejection (e.g. an identifier that fails
   `safeIdentRe`), and the isolation case above.

## Skeleton

```go
//go:build integration

package postgres

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

func TestXxxRepository_Create(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)            // helper: connects + returns *pgxpool.Pool
	tenantA := setupTenant(t, pool) // helper: CreateSchema + t.Cleanup(drop)

	tests := []struct {
		name    string
		input   /* ... */ struct{}
		wantErr bool
	}{
		{name: "happy path", wantErr: false},
		{name: "rejects invalid identifier", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// arrange / act / assert
		})
	}
}
```

If shared helpers (`testPool`, `setupTenant`, `dropSchema`) don't exist yet, create
them once in `helpers_test.go`. After generating, run
`go test -tags=integration ./internal/adapters/persistence/postgres/...` (or
`go vet ./...` if no DB is available) and report the result honestly.
