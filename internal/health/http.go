// Package health provides backend health checking
package health

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// shared HTTP client for health checks (connection reuse)
var healthClient = &http.Client{
	Timeout: 10 * time.Second,
	Transport: &http.Transport{
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     60 * time.Second,
		DisableKeepAlives:   false,
	},
}

// HTTPCheck performs HTTP health checks
func HTTPCheck(target, path string, timeout time.Duration) (bool, error) {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	rawURL := "http://" + target + path
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return false, fmt.Errorf("invalid health check URL: %w", err)
	}
	// Ensure scheme is http (prevent SSRF via other schemes)
	if parsedURL.Scheme != "http" {
		return false, fmt.Errorf("invalid health check URL scheme: %s", parsedURL.Scheme)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, parsedURL.String(), nil)
	if err != nil {
		return false, err
	}

	resp, err := healthClient.Do(req)
	if err != nil {
		return false, err
	}
	// Drain body to enable connection reuse. A drain failure only costs us
	// the reused connection, so it is deliberately ignored.
	_, _ = io.Copy(io.Discard, resp.Body)
	resp.Body.Close()

	return resp.StatusCode >= 200 && resp.StatusCode < 300, nil
}
