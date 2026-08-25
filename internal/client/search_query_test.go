package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// SAP Gateway v2 rejects $search ("Ungültige Systemabfrageoption angegeben").
// Free-text search on a v2 service is plain `search`.
func TestSearchQueryOptionMatchesODataVersion(t *testing.T) {
	for _, tc := range []struct {
		name    string
		isV4    bool
		want    string
		notWant string
	}{
		{"v2 uses search", false, "search=Telekom", "%24search"},
		{"v4 uses $search", true, "%24search=Telekom", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var gotQuery string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotQuery = r.URL.RawQuery
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte(`{"d":{"results":[]}}`))
			}))
			defer srv.Close()

			c := NewODataClient(srv.URL, false)
			c.isV4 = tc.isV4
			if _, err := c.GetEntitySet(context.Background(), "Suppliers",
				map[string]string{"$search": "Telekom"}); err != nil {
				t.Fatalf("request failed: %v", err)
			}

			if !strings.Contains(gotQuery, tc.want) {
				t.Errorf("query %q should contain %q", gotQuery, tc.want)
			}
			if tc.notWant != "" && strings.Contains(gotQuery, tc.notWant) {
				t.Errorf("query %q should not contain %q", gotQuery, tc.notWant)
			}
		})
	}
}
