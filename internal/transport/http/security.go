// Copyright (c) 2024 OData MCP Contributors
// SPDX-License-Identifier: MIT

package http

import (
	"crypto/subtle"
	"fmt"
	"net"
	"net/http"
	"strings"
)

// SecurityConfig holds security settings for HTTP transport
type SecurityConfig struct {
	Addr               string // HTTP server address (e.g., "localhost:8080")
	Token              string // MCP token for authentication
	TLSEnabled         bool   // Whether TLS is enabled
	TLSCert            string // Path to TLS certificate
	TLSKey             string // Path to TLS key
	AllowAllInterfaces bool   // Explicit flag to allow 0.0.0.0/::
}

// ValidateHTTPSecurity validates security configuration for HTTP transport.
// Returns an error if the configuration is insecure.
//
// Security model:
//   - Token is always required; every request must carry it (see RequireToken)
//   - Binding to all interfaces (0.0.0.0/::) requires --allow-all-interfaces
//   - TLS is optional, so a proxy in front can terminate it; --tls itself
//     needs --tls-cert and --tls-key and then really serves HTTPS
func ValidateHTTPSecurity(cfg SecurityConfig) error {
	if IsUnspecifiedAddr(cfg.Addr) && !cfg.AllowAllInterfaces {
		return fmt.Errorf("binding to all interfaces (0.0.0.0/::) requires --allow-all-interfaces flag")
	}
	if cfg.Token == "" {
		return fmt.Errorf("--mcp-token required for HTTP transport")
	}
	if cfg.TLSEnabled && (cfg.TLSCert == "" || cfg.TLSKey == "") {
		return fmt.Errorf("--tls requires --tls-cert and --tls-key")
	}
	return nil
}

// TokenHeader carries the --mcp-token. Not Authorization: with
// --forward-mcp-headers that header holds the caller's OData login.
const TokenHeader = "X-MCP-Token"

// RequireToken rejects every request that does not carry the MCP token in
// TokenHeader, or - when acceptBearer is set - as "Authorization: Bearer".
// /health stays open for container and load balancer probes.
func RequireToken(token string, acceptBearer bool, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			next.ServeHTTP(w, r)
			return
		}
		provided := r.Header.Get(TokenHeader)
		if provided == "" && acceptBearer {
			provided, _ = strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		}
		// An empty token never opens the door, even if validation was bypassed.
		if token == "" || !ValidateToken(provided, token) {
			http.Error(w, "missing or invalid MCP token", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// listenAndServe starts srv with TLS when a certificate is configured.
func listenAndServe(srv *http.Server, cfg SecurityConfig) error {
	if cfg.TLSEnabled {
		return srv.ListenAndServeTLS(cfg.TLSCert, cfg.TLSKey)
	}
	return srv.ListenAndServe()
}

// IsLoopbackAddr checks if an address is a loopback address (localhost/127.x.x.x/::1)
func IsLoopbackAddr(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr
	}

	host = strings.Trim(host, "[]")

	if host == "localhost" {
		return true
	}

	ip := net.ParseIP(host)
	if ip != nil {
		return ip.IsLoopback()
	}

	// Check for 127.x shorthand (e.g., "127.1" = "127.0.0.1")
	if strings.HasPrefix(host, "127.") {
		return true
	}

	return false
}

// IsUnspecifiedAddr checks if an address binds to all interfaces (0.0.0.0, ::, or empty)
func IsUnspecifiedAddr(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		if strings.HasPrefix(addr, ":") {
			return true
		}
		host = addr
	}

	if host == "" {
		return true
	}

	host = strings.Trim(host, "[]")

	ip := net.ParseIP(host)
	if ip != nil {
		return ip.IsUnspecified()
	}

	return false
}

// ValidateToken performs constant-time comparison of tokens to prevent timing attacks
func ValidateToken(provided, expected string) bool {
	if provided == "" && expected == "" {
		return true
	}
	return subtle.ConstantTimeCompare([]byte(provided), []byte(expected)) == 1
}
