// Package webassets embeds the built panel. Run `npm --prefix web run build`
// before `go build`; the directory ships with a placeholder index.html so the
// binary always compiles.
package webassets

import "embed"

//go:embed all:dist
var Dist embed.FS
