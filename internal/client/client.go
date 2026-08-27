// Copyright (c) 2024 OData MCP Contributors
// SPDX-License-Identifier: MIT

package client

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/zmcp/odata-mcp/internal/constants"
)

// Context key for HTTP headers passed from MCP server
type contextKey string

const HTTPHeadersContextKey contextKey = "mcp-http-headers"

// authSession holds the state a service binds to one login: the CSRF token and
// the session cookies it was issued alongside.
type authSession struct {
	csrfToken string
	cookies   []*http.Cookie
}

// ODataClient handles HTTP communication with OData services
type ODataClient struct {
	baseURL    string
	httpClient *http.Client
	cookies    map[string]string
	username   string
	password   string
	verbose    bool
	isV4       bool          // Whether the service is OData v4
	timeout    time.Duration // Budget for one whole operation (incl. CSRF fetch)

	// Sessions are keyed per identity. With --forward-mcp-headers every MCP
	// caller can authenticate as a different user, and a service binds CSRF
	// tokens and session cookies to the login they were issued for - one
	// shared set would send user A's session along with user B's request.
	sessionsMu sync.Mutex
	sessions   map[string]*authSession
}

// NewODataClient creates a new OData client
func NewODataClient(baseURL string, verbose bool) *ODataClient {
	// Ensure base URL ends with /
	if !strings.HasSuffix(baseURL, "/") {
		baseURL += "/"
	}

	timeout := time.Duration(constants.DefaultTimeout) * time.Second
	return &ODataClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: timeout,
		},
		verbose:  verbose,
		isV4:     false, // Will be determined when fetching metadata
		timeout:  timeout,
		sessions: make(map[string]*authSession),
	}
}

// SetTimeout sets the budget for a single operation. A modifying operation
// fetches a CSRF token first, so without a shared deadline the caller would
// wait 2x the timeout before seeing an error.
func (c *ODataClient) SetTimeout(d time.Duration) {
	if d <= 0 {
		return
	}
	c.timeout = d
	c.httpClient.Timeout = d
}

// opCtx bounds one whole operation (CSRF token fetch + actual request).
func (c *ODataClient) opCtx(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, c.timeout)
}

// SetBasicAuth configures basic authentication
func (c *ODataClient) SetBasicAuth(username, password string) {
	c.username = username
	c.password = password
}

// SetCookies configures cookie authentication
func (c *ODataClient) SetCookies(cookies map[string]string) {
	c.cookies = cookies
}

// sessionKey identifies the login a request authenticates as. The
// Authorization header is hashed so the key never carries the credential
// itself. Unauthenticated requests share the empty key.
func sessionKey(req *http.Request) string {
	auth := req.Header.Get(constants.Authorization)
	if auth == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(auth))
	return hex.EncodeToString(sum[:])
}

// session returns the state for key, creating it on first use.
// Callers must hold sessionsMu.
// ponytail: unbounded map, one entry per distinct credential. Add a TTL or LRU
// if this ever serves more than a handful of users.
func (c *ODataClient) session(key string) *authSession {
	if c.sessions == nil {
		c.sessions = make(map[string]*authSession)
	}
	s, ok := c.sessions[key]
	if !ok {
		s = &authSession{}
		c.sessions[key] = s
	}
	return s
}

// csrfTokenFor returns the CSRF token held for an identity, if any.
func (c *ODataClient) csrfTokenFor(key string) string {
	c.sessionsMu.Lock()
	defer c.sessionsMu.Unlock()
	return c.session(key).csrfToken
}

// setCSRFToken stores the CSRF token for an identity; an empty token clears it.
func (c *ODataClient) setCSRFToken(key, token string) {
	c.sessionsMu.Lock()
	defer c.sessionsMu.Unlock()
	c.session(key).csrfToken = token
}

// sessionCookiesFor returns the cookies the service issued for an identity.
func (c *ODataClient) sessionCookiesFor(key string) []*http.Cookie {
	c.sessionsMu.Lock()
	defer c.sessionsMu.Unlock()
	s := c.session(key)
	out := make([]*http.Cookie, len(s.cookies))
	copy(out, s.cookies)
	return out
}

// addSessionCookies records cookies the service issued for an identity.
func (c *ODataClient) addSessionCookies(key string, cookies []*http.Cookie) {
	c.sessionsMu.Lock()
	defer c.sessionsMu.Unlock()
	s := c.session(key)
	s.cookies = append(s.cookies, cookies...)
}

// Helper function for min
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// shouldForwardHeader determines if a header should be forwarded from MCP to OData service
func shouldForwardHeader(headerName string) bool {
	// Normalize to lowercase for comparison
	lower := strings.ToLower(headerName)

	// Allow authentication headers
	if lower == "authorization" || lower == "cookie" {
		return true
	}

	// Allow custom headers (X- prefix)
	if strings.HasPrefix(lower, "x-") {
		return true
	}

	// Block hop-by-hop headers and other problematic headers
	blockedHeaders := []string{
		"host",
		"connection",
		"keep-alive",
		"transfer-encoding",
		"upgrade",
		"proxy-authenticate",
		"proxy-authorization",
		"te",
		"trailer",
		"content-length", // Will be set by http.Client
		"content-type",   // Set by specific methods
		"accept",         // Set by buildRequest
		"user-agent",     // Set by buildRequest
	}

	for _, blocked := range blockedHeaders {
		if lower == blocked {
			return false
		}
	}

	// Allow other headers by default
	return true
}
