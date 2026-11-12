package bridge

import (
	"context"
	"strings"
	"testing"

	"github.com/zmcp/odata-mcp/internal/config"
	"github.com/zmcp/odata-mcp/internal/hint"
	"github.com/zmcp/odata-mcp/internal/mcp"
	"github.com/zmcp/odata-mcp/internal/models"
)

// Tool schemas never expose the "$" prefix; incoming names map back to the
// OData query option, and "$"-prefixed names still work.
func TestParameterNames(t *testing.T) {
	bridge := &ODataMCPBridge{config: &config.Config{}}

	for in, want := range map[string]string{
		"$filter": "filter", "$select": "select", "$top": "top", "$count": "count", "customParam": "customParam",
	} {
		if got := bridge.getParameterName(in); got != want {
			t.Errorf("getParameterName(%q) = %q, want %q", in, got, want)
		}
	}

	for in, want := range map[string]string{
		"filter": "$filter", "select": "$select", "expand": "$expand", "orderby": "$orderby",
		"top": "$top", "skip": "$skip", "count": "$count", "search": "$search",
		"$filter": "$filter", "customParam": "customParam",
	} {
		if got := bridge.mapParameterToOData(in); got != want {
			t.Errorf("mapParameterToOData(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestUniversalToolDescription(t *testing.T) {
	cfg := &config.Config{
		ServiceURL:    "https://example.com/odata",
		UniversalTool: true,
	}

	bridge := &ODataMCPBridge{
		config:      cfg,
		tools:       make(map[string]*models.ToolInfo),
		hintManager: hint.NewManager(),
		metadata: &models.ODataMetadata{
			EntitySets: map[string]*models.EntitySet{
				"Products": {
					Name:       "Products",
					EntityType: "Product",
					Creatable:  true,
					Updatable:  true,
					Deletable:  true,
					Searchable: true,
				},
				"Categories": {
					Name:       "Categories",
					EntityType: "Category",
					Creatable:  false,
					Updatable:  true,
					Deletable:  false,
					Searchable: false,
				},
			},
			FunctionImports: map[string]*models.FunctionImport{
				"GetTopProducts": {
					Name:       "GetTopProducts",
					HTTPMethod: "GET",
					Parameters: []*models.FunctionParameter{
						{Name: "count", Type: "Edm.Int32", Mode: "In"},
					},
				},
			},
		},
	}

	description := bridge.generateUniversalDescription()

	// Check service URL is included
	if !strings.Contains(description, "https://example.com/odata") {
		t.Error("Description should contain service URL")
	}

	// Check entities are listed
	if !strings.Contains(description, "Products") {
		t.Error("Description should list Products entity")
	}
	if !strings.Contains(description, "Categories") {
		t.Error("Description should list Categories entity")
	}

	// Check capabilities are shown
	if !strings.Contains(description, "search") {
		t.Error("Description should show search capability for Products")
	}

	// Check functions are listed
	if !strings.Contains(description, "GetTopProducts") {
		t.Error("Description should list GetTopProducts function")
	}

	// Check action examples are included
	if !strings.Contains(description, "list") || !strings.Contains(description, "get") {
		t.Error("Description should include action examples")
	}
}

func TestUniversalToolGeneration(t *testing.T) {
	cfg := &config.Config{
		ServiceURL:    "https://example.com/odata",
		UniversalTool: true,
	}

	mcpServer := mcp.NewServer("test", "1.0")

	bridge := &ODataMCPBridge{
		config:      cfg,
		server:      mcpServer,
		tools:       make(map[string]*models.ToolInfo),
		hintManager: hint.NewManager(),
		metadata: &models.ODataMetadata{
			EntitySets: map[string]*models.EntitySet{
				"Products": {
					Name:       "Products",
					EntityType: "Product",
				},
			},
			FunctionImports: map[string]*models.FunctionImport{},
		},
	}

	bridge.generateUniversalTool()

	// Should generate exactly 1 tool
	if len(bridge.tools) != 1 {
		t.Errorf("Universal mode should generate 1 tool, got %d", len(bridge.tools))
	}

	// Check the tool exists with correct name
	var toolName string
	for name := range bridge.tools {
		toolName = name
	}

	if !strings.Contains(toolName, "OData") {
		t.Errorf("Universal tool should have 'OData' in name, got %s", toolName)
	}
}

// The universal tool must honor --entities/--functions and --enable/--disable
// like the per-entity tools do. Denied calls fail before any OData request.
func TestUniversalToolHonorsFilters(t *testing.T) {
	bridge := &ODataMCPBridge{
		config: &config.Config{
			UniversalTool:    true,
			AllowedEntities:  []string{"Products"},
			AllowedFunctions: []string{"NoSuchFunction"},
			DisableOps:       "D",
		},
		metadata: &models.ODataMetadata{
			EntitySets: map[string]*models.EntitySet{
				"Products": {Name: "Products", EntityType: "Product", Deletable: true},
				"Orders":   {Name: "Orders", EntityType: "Order"},
			},
			FunctionImports: map[string]*models.FunctionImport{
				"ReleaseOrder": {Name: "ReleaseOrder", HTTPMethod: "GET"},
			},
		},
	}

	tests := []struct {
		args    map[string]any
		wantErr string
	}{
		{map[string]any{"action": "list", "target": "Orders"}, "unknown target"},
		{map[string]any{"action": "call", "target": "ReleaseOrder"}, "unknown target"},
		{map[string]any{"action": "delete", "target": "Products", "params": map[string]any{"key": map[string]any{"ID": 1}}}, "disabled"},
	}
	for _, tt := range tests {
		_, err := bridge.handleUniversalTool(context.Background(), tt.args)
		if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
			t.Errorf("%v: got error %v, want %q", tt.args, err, tt.wantErr)
		}
	}
}

func TestUniversalVsStandardToolCount(t *testing.T) {
	// Create metadata with multiple entities
	metadata := &models.ODataMetadata{
		EntitySets: map[string]*models.EntitySet{
			"Products":   {Name: "Products", EntityType: "Product", Creatable: true, Updatable: true, Deletable: true},
			"Categories": {Name: "Categories", EntityType: "Category", Creatable: true, Updatable: true, Deletable: true},
			"Orders":     {Name: "Orders", EntityType: "Order", Creatable: true, Updatable: true, Deletable: true},
		},
		EntityTypes: map[string]*models.EntityType{
			"Product":  {Name: "Product", KeyProperties: []string{"ID"}, Properties: []*models.EntityProperty{{Name: "ID", Type: "Edm.Int32", IsKey: true}}},
			"Category": {Name: "Category", KeyProperties: []string{"ID"}, Properties: []*models.EntityProperty{{Name: "ID", Type: "Edm.Int32", IsKey: true}}},
			"Order":    {Name: "Order", KeyProperties: []string{"ID"}, Properties: []*models.EntityProperty{{Name: "ID", Type: "Edm.Int32", IsKey: true}}},
		},
		FunctionImports: map[string]*models.FunctionImport{},
	}

	// Standard mode
	standardCfg := &config.Config{
		ServiceURL:    "https://example.com/odata",
		UniversalTool: false,
	}
	standardServer := mcp.NewServer("test", "1.0")
	standardBridge := &ODataMCPBridge{
		config:      standardCfg,
		server:      standardServer,
		tools:       make(map[string]*models.ToolInfo),
		hintManager: hint.NewManager(),
		metadata:    metadata,
	}
	standardBridge.generateTools()
	standardToolCount := len(standardBridge.tools)

	// Universal mode
	universalCfg := &config.Config{
		ServiceURL:    "https://example.com/odata",
		UniversalTool: true,
	}
	universalServer := mcp.NewServer("test", "1.0")
	universalBridge := &ODataMCPBridge{
		config:      universalCfg,
		server:      universalServer,
		tools:       make(map[string]*models.ToolInfo),
		hintManager: hint.NewManager(),
		metadata:    metadata,
	}
	universalBridge.generateTools()
	universalToolCount := len(universalBridge.tools)

	// Standard mode should have many more tools (1 service_info + ~5 per entity * 3 entities = ~16)
	// Universal mode should have exactly 1 tool
	if universalToolCount != 1 {
		t.Errorf("Universal mode should have 1 tool, got %d", universalToolCount)
	}

	if standardToolCount <= universalToolCount {
		t.Errorf("Standard mode (%d tools) should have more tools than universal mode (%d)", standardToolCount, universalToolCount)
	}

	t.Logf("Standard mode: %d tools, Universal mode: %d tools (reduction: %.1f%%)",
		standardToolCount, universalToolCount,
		float64(standardToolCount-universalToolCount)/float64(standardToolCount)*100)
}
