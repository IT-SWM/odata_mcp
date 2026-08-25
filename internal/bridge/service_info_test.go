package bridge

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/zmcp/odata-mcp/internal/config"
)

// service_info describes the service, not an entity set, so it must work
// without a target. It used to fail with "missing required parameter: target".
func TestServiceInfoNeedsNoTarget(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		w.Write([]byte(hangMetadata))
	}))
	defer srv.Close()

	b, err := NewODataMCPBridge(&config.Config{ServiceURL: srv.URL, Timeout: 5, UniversalTool: true})
	if err != nil {
		t.Fatalf("bridge: %v", err)
	}

	got, err := b.handleUniversalTool(context.Background(), map[string]any{"action": "service_info"})
	if err != nil {
		t.Fatalf("service_info failed: %v", err)
	}

	info, ok := got.(string)
	if !ok {
		t.Fatalf("unexpected result type %T", got)
	}
	for _, want := range []string{srv.URL, "C_MM_SupplierValueHelp", "entity_sets"} {
		if !strings.Contains(info, want) {
			t.Errorf("service_info missing %q: %s", want, info)
		}
	}
}

// A missing target on an action that needs one must say which action.
func TestMissingTargetNamesTheAction(t *testing.T) {
	b := &ODataMCPBridge{}
	_, err := b.handleUniversalTool(context.Background(), map[string]any{"action": "list"})
	if err == nil || !strings.Contains(err.Error(), `"list"`) {
		t.Errorf("error should name the action, got: %v", err)
	}
}
