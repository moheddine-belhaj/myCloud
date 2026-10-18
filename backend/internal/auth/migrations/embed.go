// Package migrations holds the auth service's database schema as goose SQL
// files. They are embedded into the binary and applied at startup.
//
// Rules:
//   - Never edit a migration that has been merged; add a new file instead.
//   - Keep migrations backward compatible (expand, then contract): during a
//     rolling update old and new pods run against the same schema.
//   - Number files with leading zeros (00002_...) so lexical order = version order.
package migrations

import "embed"

// FS contains every *.sql migration in this directory.
//
//go:embed *.sql
var FS embed.FS