// Package webassets embeds the built frontend assets into the backend binary.
package webassets

import "embed"

// FS contains the production frontend build copied in by scripts/build-release.sh.
//
//go:embed dist
var FS embed.FS
