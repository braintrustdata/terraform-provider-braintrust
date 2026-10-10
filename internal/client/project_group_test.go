package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestListProjectGroupsUsesSupportedFilterAndFindsLaterPage(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		query := r.URL.Query()
		if r.URL.Path != "/v1/project_group" || query.Get("project_group_name") != "Shared" || query.Get("name") != "" || query.Get("org_id") != "" || query.Get("limit") != "100" {
			t.Errorf("unexpected request: %s", r.URL.String())
		}
		w.Header().Set("Content-Type", "application/json")
		if query.Get("starting_after") == "" {
			groups := make([]ProjectGroup, 100)
			for i := range groups {
				groups[i] = ProjectGroup{ID: fmt.Sprintf("other-%03d", i), OrgID: "other-org", Name: "Shared"}
			}
			_ = json.NewEncoder(w).Encode(struct {
				Objects []ProjectGroup `json:"objects"`
			}{groups})
			return
		}
		if query.Get("starting_after") != "other-099" {
			t.Errorf("unexpected cursor: %s", r.URL.String())
		}
		_ = json.NewEncoder(w).Encode(struct {
			Objects []ProjectGroup `json:"objects"`
		}{[]ProjectGroup{{ID: "target-group", OrgID: "target-org", Name: "Shared"}}})
	}))
	defer server.Close()

	c, err := NewClient(server.URL, "test-key", "test")
	if err != nil {
		t.Fatal(err)
	}
	groups, err := c.ListProjectGroups(context.Background(), "target-org", "Shared")
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 1 || groups[0].ID != "target-group" || requests != 2 {
		t.Fatalf("expected target on second page; got groups=%#v requests=%d", groups, requests)
	}
}
