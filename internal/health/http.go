// Package health provides backend health checking
package health

import (
	"context"
	"io"
	"net/http"
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

	url := "http://" + target + path
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false, err
	}

	resp, err := healthClient.Do(req)
	if err != nil {
		return false, err
	}
	// Drain body to enable connection reuse
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()

	return resp.StatusCode >= 200 && resp.StatusCode < 300, nil
}
