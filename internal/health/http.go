// Package health provides backend health checking
package health

import (
	"context"
	"net/http"
	"time"
)

// HTTPCheck performs HTTP health checks
func HTTPCheck(target, path string, timeout time.Duration) (bool, error) {
	client := &http.Client{
		Timeout: timeout,
	}

	url := "http://" + target + path
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	if err != nil {
		return false, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	return resp.StatusCode >= 200 && resp.StatusCode < 300, nil
}
