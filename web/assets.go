// Package webassets embeds the lantern web UI assets into the binary.
package webassets

import "embed"

// Files contains the embedded lantern web UI assets.
//
//go:embed index.html style.css app.js
var Files embed.FS
