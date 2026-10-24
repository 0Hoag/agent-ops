package migrations

import "embed"

// FS contains the versioned SQL migrations shipped with the API binary.
//
//go:embed *.sql
var FS embed.FS
