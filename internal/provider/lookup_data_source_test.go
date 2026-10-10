package provider

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestDataSourceLookupRejectsAmbiguityAndSupportsScope(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/role":
			if r.URL.Query().Get("role_name") != "Viewer" {
				t.Errorf("unexpected role query: %s", r.URL.String())
			}
			_, _ = w.Write([]byte(`{"objects":[{"id":"system-role","name":"Viewer","org_id":null},{"id":"custom-role","name":"Viewer","org_id":"org-a"}]}`))
		case "/v1/role/system-role":
			_, _ = w.Write([]byte(`{"id":"system-role","name":"Viewer","org_id":null}`))
		case "/v1/project":
			if r.URL.Query().Get("project_name") != "Shared" {
				t.Errorf("unexpected project query: %s", r.URL.String())
			}
			_, _ = w.Write([]byte(`{"objects":[{"id":"project-a","name":"Shared","org_id":"org-a"},{"id":"project-b","name":"Shared","org_id":"org-b"}]}`))
		case "/v1/dataset":
			if r.URL.Query().Get("dataset_name") != "Shared" || r.URL.Query().Get("project_id") != "project-a" {
				t.Errorf("unexpected dataset query: %s", r.URL.String())
			}
			_, _ = w.Write([]byte(`{"objects":[{"id":"deleted-dataset","name":"Shared","project_id":"project-a","deleted_at":"2026-01-01"},{"id":"dataset-a","name":"Shared","project_id":"project-a"}]}`))
		case "/v1/prompt":
			if r.URL.Query().Get("prompt_name") != "Shared" || r.URL.Query().Get("project_id") != "project-a" {
				t.Errorf("unexpected prompt query: %s", r.URL.String())
			}
			_, _ = w.Write([]byte(`{"objects":[{"id":"prompt-a","name":"Shared","project_id":"project-a"},{"id":"prompt-b","name":"Shared","project_id":"project-a"}]}`))
		default:
			t.Errorf("unexpected request: %s", r.URL.String())
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	prefix := fmt.Sprintf("provider \"braintrust\" {\n  api_key = \"fixture\"\n  api_url = %q\n}\n", server.URL)
	tests := []struct {
		name      string
		config    string
		wantError string
		check     resource.TestCheckFunc
	}{
		{"ambiguous role", `data "braintrust_role" "test" { name = "Viewer" }`, "Ambiguous Role", nil},
		{"scoped role", "data \"braintrust_role\" \"test\" {\n  name = \"Viewer\"\n  org_id = \"org-a\"\n}", "", resource.TestCheckResourceAttr("data.braintrust_role.test", "id", "custom-role")},
		{"system role by ID", `data "braintrust_role" "test" { id = "system-role" }`, "", resource.TestCheckResourceAttr("data.braintrust_role.test", "name", "Viewer")},
		{"ambiguous project", `data "braintrust_project" "test" { name = "Shared" }`, "Ambiguous Project", nil},
		{"scoped project", "data \"braintrust_project\" \"test\" {\n  name = \"Shared\"\n  org_id = \"org-a\"\n}", "", resource.TestCheckResourceAttr("data.braintrust_project.test", "id", "project-a")},
		{"active dataset", "data \"braintrust_dataset\" \"test\" {\n  name = \"Shared\"\n  project_id = \"project-a\"\n}", "", resource.TestCheckResourceAttr("data.braintrust_dataset.test", "id", "dataset-a")},
		{"ambiguous prompt", "data \"braintrust_prompt\" \"test\" {\n  name = \"Shared\"\n  project_id = \"project-a\"\n}", "Ambiguous Prompt", nil},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			step := resource.TestStep{Config: prefix + test.config, Check: test.check}
			if test.wantError != "" {
				step.ExpectError = regexp.MustCompile(test.wantError)
			}
			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Steps:                    []resource.TestStep{step},
			})
		})
	}
}
