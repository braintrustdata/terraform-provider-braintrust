package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestListGroupsFiltersNameAndOrganization(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/group" || r.URL.Query().Get("group_name") != "Engineers" || r.URL.Query().Get("org_id") != "org-1" {
			t.Errorf("unexpected request: %s", r.URL.String())
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"objects":[{"id":"group-1","org_id":"org-1","name":"Engineers"}]}`))
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
}
