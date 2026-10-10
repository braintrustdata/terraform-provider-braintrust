package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestListGroupsFiltersNameAndOrganization(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		query := r.URL.Query()
		if r.URL.Path != "/v1/group" || query.Get("group_name") != "Engineers" || query.Get("org_id") != "" || query.Get("limit") != "100" {
			t.Errorf("unexpected request: %s", r.URL.String())
		}
		w.Header().Set("Content-Type", "application/json")
		if query.Get("starting_after") == "" {
			groups := make([]Group, 100)
			for i := range groups {
				groups[i] = Group{ID: fmt.Sprintf("other-%03d", i), OrgID: "other-org", Name: "Engineers"}
			}
			_ = json.NewEncoder(w).Encode(ListGroupsResponse{Objects: groups})
			return
		}
		if query.Get("starting_after") != "other-099" {
			t.Errorf("unexpected cursor: %s", r.URL.String())
		}
		_ = json.NewEncoder(w).Encode(ListGroupsResponse{Objects: []Group{{ID: "group-1", OrgID: "org-1", Name: "Engineers"}}})
	}))
	defer server.Close()

	c, err := NewClient(server.URL, "test-key", "test")
	if err != nil {
		t.Fatal(err)
	}
	groups, err := c.ListGroups(context.Background(), "Engineers", "org-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 1 || groups[0].ID != "group-1" {
		t.Fatalf("unexpected groups: %#v", groups)
	}
	if requests != 2 {
		t.Fatalf("expected two pages, got %d requests", requests)
	}
}
