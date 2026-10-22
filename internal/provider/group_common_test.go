package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/braintrustdata/terraform-provider-braintrust/internal/client"
)

func TestListGroupsFiltersNameAndOrganization(t *testing.T) {
	const otherOrg = "00000000-0000-0000-0000-0000000000bb"
	const org = "00000000-0000-0000-0000-0000000000aa"
	const want = "00000000-0000-0000-0000-000000000001"
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		query := r.URL.Query()
		if r.URL.Path != "/v1/group" || query.Get("group_name") != "Engineers" || query.Get("org_id") != "" || query.Get("limit") != "100" {
			t.Errorf("unexpected request: %s", r.URL.String())
		}
		w.Header().Set("Content-Type", "application/json")
		if query.Get("starting_after") == "" {
			groups := make([]map[string]string, 100)
			for i := range groups {
				groups[i] = map[string]string{"id": fmt.Sprintf("00000000-0000-0000-0001-%012d", i), "org_id": otherOrg, "name": "Engineers"}
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"objects": groups})
			return
		}
		if query.Get("starting_after") != "00000000-0000-0000-0001-000000000099" {
			t.Errorf("unexpected cursor: %s", r.URL.String())
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"objects": []map[string]string{{"id": want, "org_id": org, "name": "Engineers"}}})
	}))
	defer server.Close()

	c, err := client.NewClient(server.URL, "test-key", "test")
	if err != nil {
		t.Fatal(err)
	}
	groups, err := listGroups(context.Background(), c, "Engineers", org)
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 1 || groups[0].Id.String() != want {
		t.Fatalf("unexpected groups: %#v", groups)
	}
	if requests != 2 {
		t.Fatalf("expected two pages, got %d requests", requests)
	}
}
