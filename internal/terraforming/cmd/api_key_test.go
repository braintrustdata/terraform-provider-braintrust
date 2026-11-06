package cmd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/braintrustdata/terraform-provider-braintrust/internal/terraforming/api"
	"github.com/braintrustdata/terraform-provider-braintrust/internal/terraforming/hcl"
	"github.com/braintrustdata/terraform-provider-braintrust/internal/terraforming/schema"
)

func TestAPIKeyExportScopesMetadataAndSupportsImport(t *testing.T) {
	orgID, otherOrgID := fixtureID(1).String(), fixtureID(2).String()
	keyID := fixtureID(3).String()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/api_key" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"objects": []map[string]any{
			{"id": keyID, "name": "export-key", "org_id": orgID, "preview_name": "sk-preview", "key": "never-export-this-secret", "expires_at": "2099-01-01T00:00:00Z"},
			{"id": fixtureID(4).String(), "name": "other-org-key", "org_id": otherOrgID},
			{"id": fixtureID(5).String(), "name": "unscoped-key", "org_id": nil},
			{"id": fixtureID(6).String(), "name": "unknown-org-key"},
		}})
	}))
	defer server.Close()
	previous := apiClient
	t.Cleanup(func() { apiClient = previous })
	var err error
	apiClient, err = api.NewClient(server.URL, "fixture", "")
	if err != nil {
		t.Fatal(err)
	}
	blocks, err := buildAPIKeyBlocks(context.Background(), orgID)
	if err != nil {
		t.Fatal(err)
	}
	if len(blocks) != 1 || blocks[0].ID != keyID {
		t.Fatalf("expected only selected org key, got %#v", blocks)
	}
	if len(blocks[0].Fields) != 2 || blocks[0].Fields["name"] != "export-key" || blocks[0].Fields["org_id"] != orgID {
		t.Fatalf("export must contain only name and org_id: %#v", blocks[0].Fields)
	}
	registry, err := schema.LoadRegistry()
	if err != nil {
		t.Fatal(err)
	}
	writer := hcl.NewWriter(registry)
	resources := string(writer.WriteResources(blocks))
	if !strings.Contains(resources, `resource "braintrust_api_key"`) || !strings.Contains(resources, `"export-key"`) || !strings.Contains(resources, orgID) {
		t.Fatalf("API key configuration missing: %s", resources)
	}
	for _, forbidden := range []string{"expires_in_seconds", "expires_at", "preview_name", "never-export-this-secret", "sk-preview", "other-org-key", "unscoped-key", "unknown-org-key"} {
		if strings.Contains(resources, forbidden) {
			t.Errorf("API key configuration contains %q", forbidden)
		}
	}
	importable := importableBlocks(append(blocks, hcl.ResourceBlock{Type: "braintrust_service_token", ID: fixtureID(7).String()}))
	if len(importable) != 1 || importable[0].Type != "braintrust_api_key" {
		t.Fatalf("API key metadata import was not retained: %#v", importable)
	}
	imports := string(writer.WriteImports(importable))
	if !strings.Contains(imports, "to = braintrust_api_key.") || !strings.Contains(imports, keyID) || strings.Contains(imports, "braintrust_service_token") {
		t.Fatalf("API key import output incorrect: %s", imports)
	}
}
