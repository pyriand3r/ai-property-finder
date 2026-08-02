// Package migrations embeds the SQL migration files so they ship inside the
// compiled backend binary and can be applied on startup without a separate
// filesystem step.
package migrations

import "embed"

// Files embeds all *.sql migration files in this directory (both *.up.sql and
// *.down.sql). golang-migrate reads them via its iofs source driver.
//
//go:embed *.sql
var Files embed.FS
