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

func TestOrgAutomationLifecycle(t *testing.T) {
	var mu sync.Mutex
	var current *client.OrgAutomation
	creates, updates, deletes := 0, 0, 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/org_automation":
			var body client.CreateOrgAutomationRequest
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("decode create: %v", err)
			}
			if body.OrgID != "test-org" || body.Config.EventType != "retention" || body.Config.ObjectType != "project_logs" {
				t.Errorf("bad create request: %+v", body)
			}
			creates++
			current = &client.OrgAutomation{ID: "policy-id", OrgID: body.OrgID, Name: body.Name, Description: body.Description, Config: body.Config}
			_ = json.NewEncoder(w).Encode(current)
		case r.Method == http.MethodGet && r.URL.Path == "/v1/org_automation/policy-id":
			if current == nil {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			_ = json.NewEncoder(w).Encode(current)
		case r.Method == http.MethodPatch && r.URL.Path == "/v1/org_automation/policy-id":
			var body client.UpdateOrgAutomationRequest
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("decode update: %v", err)
			}
			if body.Config.EventType != "retention" || body.Config.RetentionDays != 30 || body.Description != nil {
				t.Errorf("bad update request: %+v", body)
			}
			updates++
			current.Name, current.Description, current.Config = body.Name, body.Description, body.Config
			_ = json.NewEncoder(w).Encode(current)
		case r.Method == http.MethodDelete && r.URL.Path == "/v1/org_automation/policy-id":
			deletes++
			current = nil
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.String())
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	config := func(days int, description string) string {
		return fmt.Sprintf(`
provider "braintrust" {
  api_key = "fixture"
  api_url = %q
}
resource "braintrust_org_automation" "retention" {
  org_id = "test-org"
  name = "Log retention"
  %s
  retention_config = {
    object_type = "project_logs"
    retention_days = %d
  }
}
`, server.URL, description, days)
	}
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: config(90, `description = "Initial policy"`), Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr("braintrust_org_automation.retention", "id", "policy-id"),
				resource.TestCheckResourceAttr("braintrust_org_automation.retention", "retention_config.retention_days", "90"),
			)},
			{ResourceName: "braintrust_org_automation.retention", ImportState: true, ImportStateVerify: true},
			{Config: config(30, ""), Check: resource.TestCheckResourceAttr("braintrust_org_automation.retention", "retention_config.retention_days", "30")},
		},
	})
	mu.Lock()
	defer mu.Unlock()
	if creates != 1 || updates != 1 || deletes != 1 || current != nil {
		t.Fatalf("expected create, update, import, and destroy; creates=%d updates=%d deletes=%d current=%+v", creates, updates, deletes, current)
	}
}
