package bridge

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/zmcp/odata-mcp/internal/config"
)

const hangMetadata = `<?xml version="1.0" encoding="utf-8"?>
<edmx:Edmx Version="1.0" xmlns:edmx="http://schemas.microsoft.com/ado/2007/06/edmx" xmlns:sap="http://www.sap.com/Protocols/SAPData">
 <edmx:DataServices xmlns:m="http://schemas.microsoft.com/ado/2007/08/dataservices/metadata" m:DataServiceVersion="2.0">
  <Schema Namespace="TEST" xmlns="http://schemas.microsoft.com/ado/2008/09/edm">
   <EntityType Name="SupplierType">
    <Key><PropertyRef Name="Supplier"/></Key>
    <Property Name="Supplier" Type="Edm.String" Nullable="false"/>
    <Property Name="SupplierName" Type="Edm.String"/>
   </EntityType>
   <EntityContainer Name="Container" m:IsDefaultEntityContainer="true">
    <EntitySet Name="C_MM_SupplierValueHelp" EntityType="TEST.SupplierType" sap:searchable="true"/>
   </EntityContainer>
  </Schema>
 </edmx:DataServices>
</edmx:Edmx>`

// TestSearchAgainstHangingServiceRespectsTimeout: the MCP client must get an
// error at the configured timeout, not hang until it cancels the call itself.
func TestSearchAgainstHangingServiceRespectsTimeout(t *testing.T) {
	shutdown := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "$metadata") {
			w.Header().Set("Content-Type", "application/xml")
			w.Write([]byte(hangMetadata))
			return
		}
		select {
		case <-r.Context().Done():
		case <-shutdown:
		}
	}))
	defer srv.Close()
	defer close(shutdown)

	b, err := NewODataMCPBridge(&config.Config{ServiceURL: srv.URL, Timeout: 1, UniversalTool: true})
	if err != nil {
		t.Fatalf("bridge: %v", err)
	}

	start := time.Now()
	_, err = b.handleUniversalTool(context.Background(), map[string]any{
		"action": "search",
		"target": "C_MM_SupplierValueHelp",
		"params": map[string]any{"search": "Telekom", "top": float64(20)},
	})
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected timeout error")
	}
	if elapsed > 1500*time.Millisecond {
		t.Errorf("search took %v, expected ~1s", elapsed)
	}
	t.Logf("failed after %v: %v", elapsed, err)
}
