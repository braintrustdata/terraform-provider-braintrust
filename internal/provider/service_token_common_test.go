package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/braintrustdata/terraform-provider-braintrust/internal/client"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestListServiceTokensByNamePaginates(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		requests++
		if req.URL.Path != "/v1/service_token" || req.URL.Query().Get("name") != "fixture" || req.URL.Query().Get("limit") != "100" {
			t.Errorf("unexpected list request: %s", req.URL.String())
		}
		objects := make([]map[string]string, 0, 100)
		token := func(i int) map[string]string {
			return map[string]string{"id": fmt.Sprintf("00000000-0000-0000-0001-%012d", i), "name": "fixture", "preview_name": "bt-st-…"}
		}
		if requests == 1 {
			for i := 0; i < 100; i++ {
				objects = append(objects, token(i))
			}
		} else if req.URL.Query().Get("starting_after") != "00000000-0000-0000-0001-000000000099" {
			t.Errorf("wrong cursor: %s", req.URL.String())
		} else {
			objects = append(objects, token(100))
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"objects": objects})
	}))
	defer server.Close()
	c, err := client.NewClient(server.URL, "fixture-key", "")
	if err != nil {
		t.Fatal(err)
	}
	tokens, err := listServiceTokensByName(context.Background(), c, "fixture")
	if err != nil {
		t.Fatal(err)
	}
	if requests != 2 || len(tokens) != 101 || tokens[100].Id.String() != "00000000-0000-0000-0001-000000000100" {
		t.Fatalf("got %d requests and %d tokens", requests, len(tokens))
	}
}

// A POST that commits but whose body is not decoded (here a 201, which the
// SDK does not decode) must reconcile through the list endpoint, not fail
// plainly and leave the new token orphaned.
func TestServiceTokenCreateUndecodedBodyReconciles(t *testing.T) {
	const (
		tokenID   = "00000000-0000-0000-0001-000000000001"
		accountID = "00000000-0000-0000-0002-000000000001"
	)
	created, deletes := false, 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/service_token":
			objects := []map[string]string{}
			if created {
				objects = append(objects, map[string]string{"id": tokenID, "name": "fixture", "preview_name": "bt-st-…", "service_account_id": accountID})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"objects": objects})
		case r.Method == http.MethodPost && r.URL.Path == "/v1/service_token":
			created = true
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":"` + tokenID + `","name":"fixture","key":"bt-st-secret"}`))
		case r.Method == http.MethodDelete && r.URL.Path == "/v1/service_token/"+tokenID:
			deletes++
			_, _ = w.Write([]byte(`{"id":"` + tokenID + `","name":"fixture"}`))
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
resource "braintrust_service_token" "test" {
  name               = "fixture"
  service_account_id = %q
}
`, server.URL, accountID)
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{{
			Config:      config,
			ExpectError: regexp.MustCompile("Service Token Created But Key Was Lost"),
		}},
	})
	if deletes != 1 {
		t.Fatalf("the reconciled token must be kept in state and destroyed; deletes=%d", deletes)
	}
}
