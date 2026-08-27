package test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zmcp/odata-mcp/internal/client"
	"github.com/zmcp/odata-mcp/internal/constants"
)

// ctxWithAuth builds the context the streamable-http transport creates when
// --forward-mcp-headers is on and the MCP caller sent credentials.
func ctxWithAuth(auth string) context.Context {
	headers := http.Header{}
	headers.Set(constants.Authorization, auth)
	return context.WithValue(context.Background(), client.HTTPHeadersContextKey, headers)
}

// TestForwardedAuthBeatsConfiguredAuth covers the per-user setup: a service
// account is configured for the startup metadata fetch, but a request carrying
// a forwarded Authorization header must reach the service as that end user.
func TestForwardedAuthBeatsConfiguredAuth(t *testing.T) {
	var seen []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, _, _ := r.BasicAuth()
		seen = append(seen, user)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"d": map[string]interface{}{"results": []interface{}{}}})
	}))
	defer server.Close()

	c := client.NewODataClient(server.URL, false)
	c.SetBasicAuth("svcuser", "svcpass")

	// Forwarded credentials win over the configured service account.
	_, err := c.GetEntitySet(ctxWithAuth("Basic YWxpY2U6c2VjcmV0"), "TestEntities", nil) // alice:secret
	require.NoError(t, err)

	// Without forwarded credentials the service account still applies.
	_, err = c.GetEntitySet(context.Background(), "TestEntities", nil)
	require.NoError(t, err)

	require.Len(t, seen, 2)
	assert.Equal(t, "alice", seen[0], "forwarded Authorization header must not be overridden by config auth")
	assert.Equal(t, "svcuser", seen[1], "config auth must still apply when no header is forwarded")
}

// TestUsernamePasswordHeadersBuildBasicAuth covers clients (e.g. Obot) that
// can't construct a Basic Authorization header themselves and instead send
// credentials as two plain headers.
func TestUsernamePasswordHeadersBuildBasicAuth(t *testing.T) {
	var seenUser, seenPass string
	var sawRawHeaders bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenUser, seenPass, _ = r.BasicAuth()
		if r.Header.Get(constants.HeaderUsername) != "" || r.Header.Get(constants.HeaderPassword) != "" {
			sawRawHeaders = true
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"d": map[string]interface{}{"results": []interface{}{}}})
	}))
	defer server.Close()

	c := client.NewODataClient(server.URL, false)

	headers := http.Header{}
	headers.Set(constants.HeaderUsername, "alice")
	headers.Set(constants.HeaderPassword, "secret")
	ctx := context.WithValue(context.Background(), client.HTTPHeadersContextKey, headers)

	_, err := c.GetEntitySet(ctx, "TestEntities", nil)
	require.NoError(t, err)

	assert.Equal(t, "alice", seenUser)
	assert.Equal(t, "secret", seenPass)
	assert.False(t, sawRawHeaders, "raw username/password headers must not reach the OData service")
}

// TestSessionStateIsPerIdentity covers the isolation the shared client used to
// break: a CSRF token and the session cookie it was issued with belong to one
// login and must never travel with another user's request.
func TestSessionStateIsPerIdentity(t *testing.T) {
	// Captured inside the handler: an *http.Request is not valid once the
	// handler returns.
	type modifyCall struct {
		csrfToken string
		cookies   []*http.Cookie
	}
	var mu sync.Mutex
	modifyCalls := []modifyCall{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, _, _ := r.BasicAuth()

		if r.Header.Get(constants.CSRFTokenHeader) == constants.CSRFTokenFetch {
			// Hand out a token and a session cookie bound to this login.
			w.Header().Set(constants.CSRFTokenHeader, "token-for-"+user)
			http.SetCookie(w, &http.Cookie{Name: "SAP_SESSIONID", Value: "session-for-" + user})
			w.WriteHeader(http.StatusOK)
			return
		}

		if r.Method == http.MethodPost {
			mu.Lock()
			modifyCalls = append(modifyCalls, modifyCall{
				csrfToken: r.Header.Get(constants.CSRFTokenHeader),
				cookies:   r.Cookies(),
			})
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(map[string]interface{}{"d": map[string]interface{}{"ID": "1"}})
			return
		}

		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "{}")
	}))
	defer server.Close()

	c := client.NewODataClient(server.URL, false)

	// alice:secret then bob:secret, each through their own forwarded header.
	_, err := c.CreateEntity(ctxWithAuth("Basic YWxpY2U6c2VjcmV0"), "TestEntities", map[string]interface{}{"Name": "a"})
	require.NoError(t, err)
	_, err = c.CreateEntity(ctxWithAuth("Basic Ym9iOnNlY3JldA=="), "TestEntities", map[string]interface{}{"Name": "b"})
	require.NoError(t, err)

	require.Len(t, modifyCalls, 2)

	for i, want := range []string{"alice", "bob"} {
		call := modifyCalls[i]

		assert.Equal(t, "token-for-"+want, call.csrfToken,
			"%s must use their own CSRF token", want)

		require.Len(t, call.cookies, 1, "%s must carry exactly their own session cookie", want)
		assert.Equal(t, "session-for-"+want, call.cookies[0].Value,
			"%s must not receive another user's session cookie", want)
	}
}
