package provider

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// A successful POST may mean the API returned an existing object. Terraform
// must leave that object unmanaged so a later destroy cannot remove it.
func TestCreateExistingProjectIsNotAdopted(t *testing.T) {
	creates, deletes := 0, 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/project":
			creates++
			w.Header().Set("x-bt-found-existing", "true")
			_, _ = w.Write([]byte(`{"id":"00000000-0000-0000-0000-000000000001","name":"existing","org_id":"00000000-0000-0000-0000-000000000002"}`))
		case r.Method == http.MethodDelete:
			deletes++
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.String())
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	config := fmt.Sprintf(`
provider "braintrust" {
  api_key = "fixture"
  api_url = %q
}
resource "braintrust_project" "existing" {
  name = "existing"
}
`, server.URL)
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{{
			Config:      config,
			ExpectError: regexp.MustCompile("already exists; import"),
		}},
	})
	if creates != 1 || deletes != 0 {
		t.Fatalf("existing object must not be adopted or deleted; creates=%d deletes=%d", creates, deletes)
	}
}

// These POST endpoints return an existing match without x-bt-found-existing,
// so the provider must detect it from a list made before the create.
func TestCreateExistingWithoutHeaderIsNotAdopted(t *testing.T) {
	const (
		existingID = "00000000-0000-0000-0000-000000000001"
		orgID      = "00000000-0000-0000-0000-000000000002"
		projectID  = "00000000-0000-0000-0000-000000000003"
		groupID    = "00000000-0000-0000-0000-000000000004"
	)
	cases := []struct {
		name, path, object, config string
		wantCreates                int
	}{
		{
			name: "group", path: "/v1/group", wantCreates: 1,
			object: `{"id":"` + existingID + `","org_id":"` + orgID + `","name":"existing"}`,
			config: `resource "braintrust_group" "existing" { name = "existing" }`,
		},
		{
			name: "role", path: "/v1/role", wantCreates: 1,
			object: `{"id":"` + existingID + `","org_id":"` + orgID + `","name":"existing"}`,
			config: `resource "braintrust_role" "existing" { name = "existing" }`,
		},
		{
			name: "acl", path: "/v1/acl", wantCreates: 1,
			object: `{"id":"` + existingID + `","object_type":"project","object_id":"` + projectID + `","group_id":"` + groupID + `","permission":"read","_object_org_id":"` + orgID + `"}`,
			config: `resource "braintrust_acl" "existing" {
  object_type = "project"
  object_id   = "` + projectID + `"
  group_id    = "` + groupID + `"
  permission  = "read"
}`,
		},
		{
			// POST overwrites a function, so the provider must not send it.
			name: "function", path: "/v1/function", wantCreates: 0,
			object: `{"id":"` + existingID + `","org_id":"` + orgID + `","project_id":"` + projectID + `","log_id":"p","name":"existing","slug":"existing","function_data":{"type":"prompt"},"_xact_id":"1"}`,
			config: `resource "braintrust_function" "existing" {
  project_id         = "` + projectID + `"
  name               = "replacement"
  slug               = "existing"
  function_data_json = jsonencode({ type = "prompt" })
}`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			creates, deletes := 0, 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch {
				case r.Method == http.MethodGet && r.URL.Path == tc.path:
					_, _ = w.Write([]byte(`{"objects":[` + tc.object + `]}`))
				case r.Method == http.MethodPost && r.URL.Path == tc.path:
					creates++
					_, _ = w.Write([]byte(tc.object))
				case r.Method == http.MethodDelete:
					deletes++
					w.WriteHeader(http.StatusNoContent)
				default:
					t.Errorf("unexpected request: %s %s", r.Method, r.URL.String())
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			defer server.Close()

			config := fmt.Sprintf(`
provider "braintrust" {
  api_key = "fixture"
  api_url = %q
}
%s
`, server.URL, tc.config)
			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Steps: []resource.TestStep{{
					Config:      config,
					ExpectError: regexp.MustCompile("already exists; import"),
				}},
			})
			if creates != tc.wantCreates || deletes != 0 {
				t.Fatalf("existing object must not be adopted or deleted; creates=%d deletes=%d", creates, deletes)
			}
		})
	}
}
