// Package web embeds the front end.
package web

import "embed"

//go:embed index.html app.js vttview.js style.css
var FS embed.FS
