// Package daytradergo embeds the files that ship inside the binary.
package daytradergo

import "embed"

// Migrations holds migrations/*.sql, applied in name order by the seed command (buildDBTables).
//
//go:embed migrations/*.sql
var Migrations embed.FS

// Templates holds every html/template file. Each feature area adds its own
// templates/<area>_<page>.html; nobody edits another area's file.
//
//go:embed templates/*.html
var Templates embed.FS

// Static holds the legacy static files (style.css, images, index.html frames), served under /daytrader/.
//
//go:embed static
var Static embed.FS
