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
			_, _ = w.Write([]byte(`{"id":"existing-project","name":"existing","org_id":"org"}`))
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
