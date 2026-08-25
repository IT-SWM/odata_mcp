package mcp

import (
	"context"
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
