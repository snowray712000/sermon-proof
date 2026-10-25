// Package web embeds the front end.
package web

import "embed"

//go:embed index.html app.js style.css
var FS embed.FS
