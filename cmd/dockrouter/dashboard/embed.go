// Package dashboard provides the embedded admin web UI
package dashboard

import "embed"

//go:embed index.html app.js style.css
var Files embed.FS
