// Package middleware provides HTTP middleware components
package middleware

import (
	"net"
	"net/http"
)

// IPFilter provides IP whitelist/blacklist filtering
type IPFilter struct {
	whitelist []*net.IPNet
	blacklist []*net.IPNet
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

// Middleware returns IP filtering middleware
func (f *IPFilter) Middleware() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := extractIP(r)

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

func extractIP(r *http.Request) net.IP {
	host, _, _ := net.SplitHostPort(r.RemoteAddr)
	return net.ParseIP(host)
}
