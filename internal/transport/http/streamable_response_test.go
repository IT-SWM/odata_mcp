package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

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
