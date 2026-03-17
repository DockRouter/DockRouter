// Package middleware provides HTTP middleware components
package middleware

import (
	"net"
	"net/http"
	"strings"
)

// IPFilter provides IP whitelist/blacklist filtering
type IPFilter struct {
	whitelist      []*net.IPNet
	blacklist      []*net.IPNet
	trustedProxies []*net.IPNet
}

// NewIPFilter creates a new IP filter
func NewIPFilter() *IPFilter {
	return &IPFilter{}
}

// AddWhitelist adds a CIDR to whitelist
func (f *IPFilter) AddWhitelist(cidr string) error {
	_, network, err := net.ParseCIDR(cidr)
	if err != nil {
		return err
	}
	f.whitelist = append(f.whitelist, network)
	return nil
}

// AddBlacklist adds a CIDR to blacklist
func (f *IPFilter) AddBlacklist(cidr string) error {
	_, network, err := net.ParseCIDR(cidr)
	if err != nil {
		return err
	}
	f.blacklist = append(f.blacklist, network)
	return nil
}

// AddTrustedProxy adds a trusted proxy CIDR
func (f *IPFilter) AddTrustedProxy(cidr string) error {
	_, network, err := net.ParseCIDR(cidr)
	if err != nil {
		return err
	}
	f.trustedProxies = append(f.trustedProxies, network)
	return nil
}

// Middleware returns IP filtering middleware
func (f *IPFilter) Middleware() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := extractIP(r, f.trustedProxies)

			// Check blacklist first
			for _, network := range f.blacklist {
				if network.Contains(ip) {
					http.Error(w, "Forbidden", http.StatusForbidden)
					return
				}
			}

			// Check whitelist if configured
			if len(f.whitelist) > 0 {
				allowed := false
				for _, network := range f.whitelist {
					if network.Contains(ip) {
						allowed = true
						break
					}
				}
				if !allowed {
					http.Error(w, "Forbidden", http.StatusForbidden)
					return
				}
			}

			next.ServeHTTP(w, r)
		})
	}
}

// extractIP extracts the real client IP from the request
// It checks proxy headers if the request comes from a trusted proxy
func extractIP(r *http.Request, trustedProxies []*net.IPNet) net.IP {
	// Get the direct peer IP
	host, _, _ := net.SplitHostPort(r.RemoteAddr)
	peerIP := net.ParseIP(host)

	// If no trusted proxies configured, just use peer IP
	if len(trustedProxies) == 0 {
		return peerIP
	}

	// Check if the peer is a trusted proxy
	isTrustedProxy := false
	for _, network := range trustedProxies {
		if network.Contains(peerIP) {
			isTrustedProxy = true
			break
		}
	}

	// If not from trusted proxy, use peer IP
	if !isTrustedProxy {
		return peerIP
	}

	// Try X-Forwarded-For header (most common)
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// X-Forwarded-For: client, proxy1, proxy2
		// The first IP is the original client
		ips := strings.Split(xff, ",")
		if len(ips) > 0 {
			clientIP := strings.TrimSpace(ips[0])
			if ip := net.ParseIP(clientIP); ip != nil {
				return ip
			}
		}
	}

	// Try X-Real-IP header
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		if ip := net.ParseIP(strings.TrimSpace(xri)); ip != nil {
			return ip
		}
	}

	// Try CF-Connecting-IP (Cloudflare)
	if cfIP := r.Header.Get("CF-Connecting-IP"); cfIP != "" {
		if ip := net.ParseIP(strings.TrimSpace(cfIP)); ip != nil {
			return ip
		}
	}

	// Try True-Client-IP (Cloudflare Enterprise)
	if tcIP := r.Header.Get("True-Client-IP"); tcIP != "" {
		if ip := net.ParseIP(strings.TrimSpace(tcIP)); ip != nil {
			return ip
		}
	}

	// Fallback to peer IP
	return peerIP
}

// ExtractClientIP is a helper function to extract client IP from request
// This is useful for rate limiting and logging
func ExtractClientIP(r *http.Request) net.IP {
	return extractIP(r, nil)
}

// ExtractClientIPWithTrustedProxies extracts client IP with trusted proxy support
func ExtractClientIPWithTrustedProxies(r *http.Request, trustedProxies []*net.IPNet) net.IP {
	return extractIP(r, trustedProxies)
}
