package repository

import (
	"context"
	"database/sql"

	"github.com/pressly/goose/v3"
)

// 00042 repeats the stored-URL scrub of 00041 now that enrich.RedactURL also
// redacts user_code, the device-flow code that 00041 left in place (the
// iam.wiebe.xyz/device row of 2026-08-17). It reuses 00041, so the two cannot
// drift. The scrub is idempotent and keeps every row.
func init() {
	goose.AddMigrationContext(up00042RedactStoredUserCode, nil)
	goMigrationVersions = append(goMigrationVersions, 42)
}

func up00042RedactStoredUserCode(ctx context.Context, tx *sql.Tx) error {
	return up00041RedactStoredURLs(ctx, tx)
}
