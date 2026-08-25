package mcp

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// A panicking tool handler must produce a JSON-RPC error, not kill the process
// (a dead process leaves the client waiting for its own timeout).
func TestPanickingToolHandlerReturnsError(t *testing.T) {
	s := NewServer("test", "0")
	s.AddTool(&Tool{Name: "boom"}, func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
		var m map[string]string
		m["x"] = "y" // assignment to nil map
		return nil, nil
	})

	msg, err := s.handleToolsCallV2(&Request{
		ID:     1,
		Params: map[string]interface{}{"name": "boom", "arguments": map[string]interface{}{}},
	})
	if err != nil {
		t.Fatalf("handler returned transport error: %v", err)
	}
	if msg.Error == nil {
		t.Fatal("expected a JSON-RPC error response")
	}
	if !strings.Contains(strings.ToLower(msg.Error.Message+string(msg.Error.Data)), "panic") {
		t.Logf("error response: %+v", msg.Error)
	}
}

// Error text routinely contains quotes (a URL, a parameter name). Quoting it by
// hand produced a broken json.RawMessage: encoding failed, nothing was written,
// and the client waited out its own timeout on an empty 200.
func TestErrorResponseWithQuotesEncodes(t *testing.T) {
	s := NewServer("test", "0")
	msg := s.createErrorResponse(json.RawMessage("1"), -32603, "Internal error",
		`Get "http://sap/Suppliers?$search=x": context deadline exceeded`)

	out, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("error response does not encode: %v", err)
	}
	if !strings.Contains(string(out), "deadline exceeded") {
		t.Errorf("error detail lost: %s", out)
	}
}
