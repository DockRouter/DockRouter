// Package proxy handles reverse proxying to backends
package proxy

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

// IsWebSocketRequest checks if a request is a WebSocket upgrade
func IsWebSocketRequest(r *http.Request) bool {
	upgrade := r.Header.Get("Upgrade")
	connection := r.Header.Get("Connection")

	return strings.ToLower(upgrade) == "websocket" &&
		strings.Contains(strings.ToLower(connection), "upgrade")
}

// WebSocketProxy handles WebSocket connections
type WebSocketProxy struct {
	dialer *net.Dialer
	logger Logger
}

// NewWebSocketProxy creates a new WebSocket proxy
func NewWebSocketProxy(logger Logger) *WebSocketProxy {
	return &WebSocketProxy{
		dialer: &net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 30 * time.Second,
		},
		logger: logger,
	}
}

// ServeHTTP handles WebSocket upgrade and proxying
func (wp *WebSocketProxy) ServeHTTP(w http.ResponseWriter, r *http.Request, target string) error {
	// Check if hijacker is supported
	hijacker, ok := w.(http.Hijacker)
	if !ok {
		return fmt.Errorf("hijacking not supported")
	}

	// Dial backend
	backendConn, err := wp.dialer.Dial("tcp", target)
	if err != nil {
		return fmt.Errorf("failed to connect to backend: %w", err)
	}
	defer backendConn.Close()

	// Hijack client connection
	clientConn, clientBuf, err := hijacker.Hijack()
	if err != nil {
		return fmt.Errorf("failed to hijack connection: %w", err)
	}
	defer clientConn.Close()

	// Send upgrade request to backend
	if err := wp.sendUpgradeRequest(backendConn, r, target); err != nil {
		return err
	}

	// Read backend response
	backendResp, err := wp.readBackendResponse(backendConn)
	if err != nil {
		return err
	}

	// Send response to client
	if err := wp.sendClientResponse(clientConn, backendResp); err != nil {
		return err
	}

	// Copy data bidirectionally
	go wp.copyData(backendConn, clientBuf, "backend->client")
	wp.copyData(clientConn, backendConn, "client->backend")

	return nil
}

func (wp *WebSocketProxy) sendUpgradeRequest(conn net.Conn, r *http.Request, target string) error {
	// Build upgrade request
	req := fmt.Sprintf("GET %s HTTP/1.1\r\n", r.URL.Path)
	req += fmt.Sprintf("Host: %s\r\n", r.Host)
	req += "Upgrade: websocket\r\n"
	req += "Connection: Upgrade\r\n"

	// WebSocket key
	if key := r.Header.Get("Sec-WebSocket-Key"); key != "" {
		req += fmt.Sprintf("Sec-WebSocket-Key: %s\r\n", key)
	}
	if version := r.Header.Get("Sec-WebSocket-Version"); version != "" {
		req += fmt.Sprintf("Sec-WebSocket-Version: %s\r\n", version)
	}
	if proto := r.Header.Get("Sec-WebSocket-Protocol"); proto != "" {
		req += fmt.Sprintf("Sec-WebSocket-Protocol: %s\r\n", proto)
	}
	if ext := r.Header.Get("Sec-WebSocket-Extensions"); ext != "" {
		req += fmt.Sprintf("Sec-WebSocket-Extensions: %s\r\n", ext)
	}

	// Origin
	if origin := r.Header.Get("Origin"); origin != "" {
		req += fmt.Sprintf("Origin: %s\r\n", origin)
	}

	req += "\r\n"

	_, err := conn.Write([]byte(req))
	return err
}

func (wp *WebSocketProxy) readBackendResponse(conn net.Conn) (string, error) {
	buf := make([]byte, 4096)
	n, err := conn.Read(buf)
	if err != nil {
		return "", err
	}
	return string(buf[:n]), nil
}

func (wp *WebSocketProxy) sendClientResponse(conn net.Conn, resp string) error {
	_, err := conn.Write([]byte(resp))
	return err
}

func (wp *WebSocketProxy) copyData(dst io.Writer, src io.Reader, direction string) {
	defer wp.logger.Debug("WebSocket copy done", "direction", direction)

	buf := make([]byte, 32*1024)
	for {
		n, err := src.Read(buf)
		if n > 0 {
			if _, writeErr := dst.Write(buf[:n]); writeErr != nil {
				wp.logger.Debug("WebSocket write error",
					"direction", direction,
					"error", writeErr,
				)
				return
			}
		}
		if err != nil {
			if err != io.EOF {
				wp.logger.Debug("WebSocket read error",
					"direction", direction,
					"error", err,
				)
			}
			return
		}
	}
}

// HijackConnection hijacks the connection for WebSocket
func HijackConnection(w http.ResponseWriter, r *http.Request) (net.Conn, *bufio.ReadWriter, error) {
	hijacker, ok := w.(http.Hijacker)
	if !ok {
		return nil, nil, fmt.Errorf("hijacking not supported")
	}

	return hijacker.Hijack()
}
