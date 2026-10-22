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
			_, _ = w.Write([]byte(`{"objects":[{"id":"00000000-0000-0000-0000-0000000000c1","name":"Viewer","org_id":null},{"id":"00000000-0000-0000-0000-0000000000c2","name":"Viewer","org_id":"00000000-0000-0000-0000-0000000000aa"}]}`))
		case "/v1/role/00000000-0000-0000-0000-0000000000c1":
			_, _ = w.Write([]byte(`{"id":"00000000-0000-0000-0000-0000000000c1","name":"Viewer","org_id":null}`))
		case "/v1/project":
			if r.URL.Query().Get("project_name") != "Shared" {
				t.Errorf("unexpected project query: %s", r.URL.String())
			}
			_, _ = w.Write([]byte(`{"objects":[{"id":"00000000-0000-0000-0000-00000000000a","name":"Shared","org_id":"00000000-0000-0000-0000-0000000000aa"},{"id":"00000000-0000-0000-0000-00000000000b","name":"Shared","org_id":"00000000-0000-0000-0000-0000000000bb"}]}`))
		case "/v1/dataset":
			if r.URL.Query().Get("dataset_name") != "Shared" || r.URL.Query().Get("project_id") != "00000000-0000-0000-0000-0000000000d0" {
				t.Errorf("unexpected dataset query: %s", r.URL.String())
			}
			_, _ = w.Write([]byte(`{"objects":[{"id":"00000000-0000-0000-0000-0000000000d1","name":"Shared","project_id":"00000000-0000-0000-0000-0000000000d0","url_slug":"shared","deleted_at":"2026-01-01T00:00:00.000Z"},{"id":"00000000-0000-0000-0000-0000000000d2","name":"Shared","project_id":"00000000-0000-0000-0000-0000000000d0","url_slug":"shared"}]}`))
		case "/v1/dataset/00000000-0000-0000-0000-0000000000d2":
			_, _ = w.Write([]byte(`{"id":"00000000-0000-0000-0000-0000000000d2","name":"Shared","project_id":"00000000-0000-0000-0000-0000000000d0","url_slug":"shared","metadata":{"owner":"team"}}`))
		case "/v1/function":
			if r.URL.Query().Get("function_name") != "Shared" || r.URL.Query().Get("project_id") != "00000000-0000-0000-0000-0000000000d0" {
				t.Errorf("unexpected function query: %s", r.URL.String())
			}
			_, _ = w.Write([]byte(`{"objects":[{"id":"00000000-0000-0000-0000-0000000000e1","name":"Shared","project_id":"00000000-0000-0000-0000-0000000000d0"},{"id":"00000000-0000-0000-0000-0000000000e2","name":"Shared","project_id":"00000000-0000-0000-0000-0000000000d0"}]}`))
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
		{"scoped role", "data \"braintrust_role\" \"test\" {\n  name = \"Viewer\"\n  org_id = \"00000000-0000-0000-0000-0000000000aa\"\n}", "", resource.TestCheckResourceAttr("data.braintrust_role.test", "id", "00000000-0000-0000-0000-0000000000c2")},
		{"system role by ID", `data "braintrust_role" "test" { id = "00000000-0000-0000-0000-0000000000c1" }`, "", resource.TestCheckResourceAttr("data.braintrust_role.test", "name", "Viewer")},
		{"ambiguous project", `data "braintrust_project" "test" { name = "Shared" }`, "Ambiguous Project", nil},
		{"scoped project", "data \"braintrust_project\" \"test\" {\n  name = \"Shared\"\n  org_id = \"00000000-0000-0000-0000-0000000000aa\"\n}", "", resource.TestCheckResourceAttr("data.braintrust_project.test", "id", "00000000-0000-0000-0000-00000000000a")},
		{"active dataset", "data \"braintrust_dataset\" \"test\" {\n  name = \"Shared\"\n  project_id = \"00000000-0000-0000-0000-0000000000d0\"\n}", "", resource.ComposeAggregateTestCheckFunc(
			resource.TestCheckResourceAttr("data.braintrust_dataset.test", "id", "00000000-0000-0000-0000-0000000000d2"),
			resource.TestCheckResourceAttr("data.braintrust_dataset.test", "metadata_json", `{"owner":"team"}`),
		)},
		{"ambiguous function", "data \"braintrust_function\" \"test\" {\n  name = \"Shared\"\n  project_id = \"00000000-0000-0000-0000-0000000000d0\"\n}", "Ambiguous Function", nil},
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
