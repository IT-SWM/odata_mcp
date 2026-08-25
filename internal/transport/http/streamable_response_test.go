package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/zmcp/odata-mcp/internal/mcp"
	"github.com/zmcp/odata-mcp/internal/transport"
)

// A tools/call must complete the HTTP response. Before this was fixed the
// handler sent the answer as an SSE event and then blocked forever, so the
// client waited out its own timeout and reported a bare cancellation.
func TestToolsCallResponseCompletes(t *testing.T) {
	tr := NewStreamableHTTP(":0", func(ctx context.Context, msg *transport.Message) (*transport.Message, error) {
		return &transport.Message{JSONRPC: "2.0", ID: msg.ID}, nil
	}, false, false)

	req := httptest.NewRequest(http.MethodPost, "/mcp",
		strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"x"}}`))
	req.Header.Set("Accept", "application/json, text/event-stream")
	rec := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		defer close(done)
		tr.handleMCP(rec, req)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("handleMCP did not return: the response stream is never closed")
	}

	if body := rec.Body.String(); !strings.Contains(body, `"id":1`) {
		t.Errorf("response body missing the result: %q", body)
	}
}

// Same thing through the real MCP server, which is what the binary wires up.
func TestToolsCallThroughMCPServerHasBody(t *testing.T) {
	srv := mcp.NewServer("test", "0")
	srv.AddTool(&mcp.Tool{Name: "ping_tool"}, func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
		return "pong", nil
	})

	tr := NewStreamableHTTP(":0", func(ctx context.Context, msg *transport.Message) (*transport.Message, error) {
		return srv.HandleMessage(ctx, msg)
	}, false, false)

	req := httptest.NewRequest(http.MethodPost, "/mcp",
		strings.NewReader(`{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"ping_tool","arguments":{}}}`))
	req.Header.Set("Accept", "application/json, text/event-stream")
	rec := httptest.NewRecorder()

	done := make(chan struct{})
	go func() { defer close(done); tr.handleMCP(rec, req) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("handleMCP did not return")
	}

	t.Logf("body: %q", rec.Body.String())
	if !strings.Contains(rec.Body.String(), "pong") {
		t.Fatal("response body is empty: the answer never reaches the client")
	}
}
