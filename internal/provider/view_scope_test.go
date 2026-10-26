package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/braintrustdata/braintrust-sdk-go/openapi"
	"github.com/braintrustdata/terraform-provider-braintrust/internal/client"
	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

const (
	viewFixtureID     = "00000000-0000-0000-0000-0000000000b1"
	viewFixtureParent = "00000000-0000-0000-0000-0000000000b2"
)

func TestViewRequestsRequireScope(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodGet:
			if r.URL.Query().Get("object_type") != "project" || r.URL.Query().Get("object_id") != viewFixtureParent {
				t.Errorf("GET missing view scope: %s", r.URL.String())
			}
			fmt.Fprintf(w, `{"id":%q,"name":"same","object_type":"project","object_id":%q,"view_type":"logs","options":{"a":1}}`, viewFixtureID, viewFixtureParent)
		case http.MethodDelete:
			var body map[string]string
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body["object_type"] != "project" || body["object_id"] != viewFixtureParent {
				t.Errorf("DELETE missing view scope: %v, %v", body, err)
			}
			fmt.Fprintf(w, `{"id":%q,"name":"same","object_type":"project","object_id":%q,"view_type":"logs"}`, viewFixtureID, viewFixtureParent)
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()
	c, err := client.NewClient(server.URL, "fixture", "")
	if err != nil {
		t.Fatal(err)
	}
	v, err := getView(context.Background(), c, viewFixtureID, "project", viewFixtureParent)
	if err != nil || string(v.Fields["options"]) != `{"a":1}` {
		t.Fatalf("get view: %v, %v", v, err)
	}
	scope := openapi.DeleteView{ObjectType: "project", ObjectId: uuid.MustParse(viewFixtureParent)}
	if _, err := client.Write(c.API.DeleteViewIdWithResponse(context.Background(), viewFixtureID, scope)); err != nil {
		t.Fatal(err)
	}
}

func TestCreateExistingViewIsNotAdopted(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.String())
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("x-bt-found-existing", "true")
		fmt.Fprintf(w, `{"id":%q,"name":"same","object_type":"project","object_id":%q,"view_type":"logs"}`, viewFixtureID, viewFixtureParent)
	}))
	defer server.Close()
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{{
			Config: fmt.Sprintf(`
provider "braintrust" {
  api_key = "fixture"
  api_url = %q
}
resource "braintrust_view" "existing" {
  object_type = "project"
  object_id   = %q
  view_type   = "logs"
  name        = "same"
}
`, server.URL, viewFixtureParent),
			ExpectError: regexp.MustCompile(`already exists;\s+import`),
		}},
	})
}
