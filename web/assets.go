// Package webassets embeds the lantern web UI assets into the binary.
package webassets

import "embed"

// Files contains the embedded lantern web UI assets.
//
//go:embed index.html about.html contact.html privacy.html style.css app.js favicon.svg robots.txt
var Files embed.FS
