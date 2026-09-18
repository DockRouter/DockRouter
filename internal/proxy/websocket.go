// Package proxy handles reverse proxying to backends
package proxy

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
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

	// Copy data bidirectionally. Whichever direction ends first closes both
	// connections: without that the surviving goroutine blocks in Read until
	// its peer happens to hang up, which for an idle WebSocket is never, and
	// the goroutine plus both sockets leak for the lifetime of the process.
	var once sync.Once
	closeBoth := func() {
		once.Do(func() {
			clientConn.Close()
			backendConn.Close()
		})
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		defer closeBoth()
		wp.copyData(clientConn, backendConn, "backend->client")
	}()
	go func() {
		defer wg.Done()
		defer closeBoth()
		wp.copyData(backendConn, clientBuf, "client->backend")
	}()
	wg.Wait()

	return nil
}

// wsHandshakeHeaders are the handshake headers sendUpgradeRequest relays with
// their conventional wire casing rather than the canonical form net/http
// stores them under ("Sec-Websocket-Key"), which some stricter servers reject.
var wsHandshakeHeaders = []string{
	"Sec-WebSocket-Key",
	"Sec-WebSocket-Version",
	"Sec-WebSocket-Protocol",
	"Sec-WebSocket-Extensions",
	"Origin",
}

// wsSkipHeaders are the headers sendUpgradeRequest writes itself or must not
// relay. Everything the client sent that is not listed here is forwarded
// verbatim: an allowlist would silently drop Cookie, Authorization and every
// application header, which breaks any backend that authenticates its
// WebSocket endpoint.
var wsSkipHeaders = map[string]bool{
	"Host":       true, // written from r.Host
	"Upgrade":    true, // rewritten below
	"Connection": true, // hop-by-hop, rewritten below

	// Hop-by-hop headers (RFC 7230 6.1) must not be forwarded.
	"Keep-Alive":          true,
	"Proxy-Authenticate":  true,
	"Proxy-Authorization": true,
	"Te":                  true,
	"Trailer":             true,
	"Transfer-Encoding":   true,

	// Set from the real connection rather than relayed, so a client cannot
	// spoof its own address.
	"X-Forwarded-For":   true,
	"X-Forwarded-Proto": true,
	"X-Forwarded-Host":  true,
	"X-Real-Ip":         true,
}

func init() {
	// Written explicitly by the handshake block, so the generic loop skips them.
	for _, h := range wsHandshakeHeaders {
		wsSkipHeaders[http.CanonicalHeaderKey(h)] = true
	}
}

func (wp *WebSocketProxy) sendUpgradeRequest(conn net.Conn, r *http.Request, target string) error {
	// Build upgrade request
	reqURI := r.URL.RequestURI()
	var req strings.Builder
	fmt.Fprintf(&req, "GET %s HTTP/1.1\r\n", reqURI)
	fmt.Fprintf(&req, "Host: %s\r\n", r.Host)
	req.WriteString("Upgrade: websocket\r\n")
	req.WriteString("Connection: Upgrade\r\n")

	// A header value carrying CRLF would let a client inject extra request
	// lines into the upstream connection.
	writeHeader := func(name, value string) {
		if strings.ContainsAny(value, "\r\n") {
			return
		}
		fmt.Fprintf(&req, "%s: %s\r\n", name, value)
	}

	for _, name := range wsHandshakeHeaders {
		if value := r.Header.Get(name); value != "" {
			writeHeader(name, value)
		}
	}

	// Forward everything else the client sent: Cookie, Authorization and any
	// application-specific header the backend needs to authorize the socket.
	for name, values := range r.Header {
		canonical := http.CanonicalHeaderKey(name)
		if wsSkipHeaders[canonical] {
			continue
		}
		for _, value := range values {
			writeHeader(canonical, value)
		}
	}

	clientIP := r.RemoteAddr
	if host, _, err := net.SplitHostPort(clientIP); err == nil {
		clientIP = host
	}
	if clientIP != "" {
		writeHeader("X-Forwarded-For", clientIP)
		writeHeader("X-Real-Ip", clientIP)
	}
	proto := "http"
	if r.TLS != nil {
		proto = "https"
	}
	writeHeader("X-Forwarded-Proto", proto)
	writeHeader("X-Forwarded-Host", r.Host)

	req.WriteString("\r\n")

	_, err := conn.Write([]byte(req.String()))
	return err
}

func (wp *WebSocketProxy) readBackendResponse(conn net.Conn) (string, error) {
	// A deadline that cannot be set is not fatal here: the read below still
	// terminates when the backend closes the connection.
	_ = conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	defer func() { _ = conn.SetDeadline(time.Time{}) }()

	reader := bufio.NewReaderSize(conn, 4096)
	var resp strings.Builder

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return "", fmt.Errorf("failed to read backend response: %w", err)
		}
		resp.WriteString(line)
		if strings.TrimSpace(line) == "" {
			break
		}
	}

	response := resp.String()
	if !strings.HasPrefix(response, "HTTP/1.1 101") && !strings.HasPrefix(response, "HTTP/1.0 101") {
		return "", fmt.Errorf("backend refused WebSocket upgrade: %s", strings.Split(response, "\r\n")[0])
	}

	return response, nil
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
