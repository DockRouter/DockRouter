// Package health provides backend health checking
package health

import (
	"net"
	"time"
)

// TCPCheck performs TCP health checks
func TCPCheck(target string, timeout time.Duration) (bool, error) {
	conn, err := net.DialTimeout("tcp", target, timeout)
	if err != nil {
		return false, err
	}
	conn.Close()
	return true, nil
}
