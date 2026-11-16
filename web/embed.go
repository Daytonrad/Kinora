// Package web embeds the vanilla JS single page client (no build step).
package web

import "embed"

// Static holds web/static (index.html, app.js, style.css, vendor/).
//
//go:embed static
var Static embed.FS
