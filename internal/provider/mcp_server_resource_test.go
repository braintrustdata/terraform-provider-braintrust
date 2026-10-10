package provider

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/braintrustdata/terraform-provider-braintrust/internal/client"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestMCPServerLifecycle(t *testing.T) {
	var mu sync.Mutex
	var current *client.MCPServer
	creates, updates, deletes := 0, 0, 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/mcp_server":
			var body client.CreateMCPServerRequest
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("decode create: %v", err)
			}
			if body.ProjectID != "test-project" || body.Name != "Tools" {
				t.Errorf("bad create: %+v", body)
			}
			creates++
			current = &client.MCPServer{ID: "mcp-id", ProjectID: body.ProjectID, Name: body.Name, URL: body.URL, Description: body.Description}
			_ = json.NewEncoder(w).Encode(current)
		case r.Method == http.MethodGet && r.URL.Path == "/v1/mcp_server/mcp-id":
			if current == nil {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			_ = json.NewEncoder(w).Encode(current)
		case r.Method == http.MethodPatch && r.URL.Path == "/v1/mcp_server/mcp-id":
			var body client.UpdateMCPServerRequest
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("decode update: %v", err)
			}
			if body.URL != "https://example.com/mcp-v2" || body.Description != nil {
				t.Errorf("bad update: %+v", body)
			}
			updates++
			current.Name, current.URL, current.Description = body.Name, body.URL, body.Description
			_ = json.NewEncoder(w).Encode(current)
		case r.Method == http.MethodDelete && r.URL.Path == "/v1/mcp_server/mcp-id":
			deletes++
			current = nil
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.String())
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	config := func(url, description string) string {
		return fmt.Sprintf(`
provider "braintrust" {
  api_key = "fixture"
  api_url = %q
}
resource "braintrust_mcp_server" "tools" {
  project_id = "test-project"
  name = "Tools"
  url = %q
  %s
}
`, server.URL, url, description)
	}
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: config("https://example.com/mcp", `description = "Initial"`), Check: resource.TestCheckResourceAttr("braintrust_mcp_server.tools", "id", "mcp-id")},
			{ResourceName: "braintrust_mcp_server.tools", ImportState: true, ImportStateVerify: true},
			{Config: config("https://example.com/mcp-v2", ""), Check: resource.TestCheckResourceAttr("braintrust_mcp_server.tools", "url", "https://example.com/mcp-v2")},
		},
	})
	mu.Lock()
	defer mu.Unlock()
	if creates != 1 || updates != 1 || deletes != 1 || current != nil {
		t.Fatalf("expected create, update, import, and destroy; creates=%d updates=%d deletes=%d current=%+v", creates, updates, deletes, current)
	}
}
