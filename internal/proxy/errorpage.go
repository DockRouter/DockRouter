// Package proxy handles reverse proxying to backends
package proxy

import (
	"html/template"
	"net/http"
)

// ErrorPageData contains data for error page templates
type ErrorPageData struct {
	StatusCode int
	StatusText string
	RequestID  string
	Message    string
}

// errorTemplates holds parsed error page templates
var errorTemplates = template.Must(template.New("error").Parse(errorPageHTML))

// RenderError renders a branded error page
func RenderError(w http.ResponseWriter, statusCode int, requestID string) {
	data := ErrorPageData{
		StatusCode: statusCode,
		StatusText: http.StatusText(statusCode),
		RequestID:  requestID,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(statusCode)
	errorTemplates.Execute(w, data)
}

const errorPageHTML = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>{{.StatusCode}} {{.StatusText}}</title>
    <style>
        body { background: #0F172A; color: #F1F5F9; font-family: system-ui; display: flex; align-items: center; justify-content: center; min-height: 100vh; margin: 0; }
        .container { text-align: center; }
        .code { font-size: 4rem; font-weight: bold; color: #F97316; }
        .message { margin: 1rem 0; color: #94A3B8; }
        .request-id { font-family: monospace; font-size: 0.875rem; color: #64748B; }
    </style>
</head>
<body>
    <div class="container">
        <div class="code">{{.StatusCode}}</div>
        <div class="message">{{.StatusText}}</div>
        <div class="request-id">Request ID: {{.RequestID}}</div>
    </div>
</body>
</html>`
