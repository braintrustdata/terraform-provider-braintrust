package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/braintrustdata/terraform-provider-braintrust/internal/client"
	"github.com/google/uuid"
	"github.com/oapi-codegen/nullable"
)

const projectGroupFixtureID = "00000000-0000-0000-0000-0000000000f1"

func TestProjectGroupMembershipPatchBatches(t *testing.T) {
	type patch struct {
		Name                 *string  `json:"name"`
		Description          *string  `json:"description"`
		AddMemberProjects    []string `json:"add_member_projects"`
		RemoveMemberProjects []string `json:"remove_member_projects"`
	}
	var batches []patch
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodPatch || req.URL.Path != "/v1/project_group/"+projectGroupFixtureID {
			t.Errorf("unexpected request: %s %s", req.Method, req.URL.Path)
			http.Error(w, "unexpected request", http.StatusBadRequest)
			return
		}
		var batch patch
		if err := json.NewDecoder(req.Body).Decode(&batch); err != nil {
			t.Errorf("decode: %v", err)
		}
		batches = append(batches, batch)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"id":%q,"org_id":"00000000-0000-0000-0000-0000000000aa","name":"renamed","member_projects":[]}`, projectGroupFixtureID)
	}))
	defer server.Close()
	c, err := client.NewClient(server.URL, "test-key", "")
	if err != nil {
		t.Fatal(err)
	}
	adds := make([]uuid.UUID, 1500)
	removes := make([]uuid.UUID, 600)
	for i := range adds {
		adds[i] = uuid.New()
	}
	for i := range removes {
		removes[i] = uuid.New()
	}
	r := &projectGroupResource{client: c}
	_, err = r.applyChanges(context.Background(), projectGroupFixtureID, projectGroupChange{
		name:        nullable.NewNullableWithValue("renamed"),
		description: nullable.NewNullableWithValue("new description"),
		add:         adds,
		remove:      removes,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(batches) != 3 {
		t.Fatalf("got %d batches, want 3", len(batches))
	}
	var gotAdds, gotRemoves []string
	for i, batch := range batches {
		if total := len(batch.AddMemberProjects) + len(batch.RemoveMemberProjects); total > 1000 || total == 0 {
			t.Errorf("batch %d has %d changes", i, total)
		}
		if (i == 0) != (batch.Name != nil && batch.Description != nil) {
			t.Errorf("metadata should appear only in first batch: %d", i)
		}
		gotAdds = append(gotAdds, batch.AddMemberProjects...)
		gotRemoves = append(gotRemoves, batch.RemoveMemberProjects...)
	}
	if len(gotAdds) != len(adds) || len(gotRemoves) != len(removes) {
		t.Errorf("lost membership changes: got %d adds and %d removes", len(gotAdds), len(gotRemoves))
	}
}

func TestListProjectGroupsUsesSupportedFilterAndFindsLaterPage(t *testing.T) {
	const otherOrg = "00000000-0000-0000-0000-0000000000bb"
	const targetOrg = "00000000-0000-0000-0000-0000000000aa"
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		query := r.URL.Query()
		if r.URL.Path != "/v1/project_group" || query.Get("project_group_name") != "Shared" || query.Get("name") != "" || query.Get("org_id") != "" || query.Get("limit") != "100" {
			t.Errorf("unexpected request: %s", r.URL.String())
		}
		w.Header().Set("Content-Type", "application/json")
		if query.Get("starting_after") == "" {
			groups := make([]map[string]any, 100)
			for i := range groups {
				groups[i] = map[string]any{"id": fmt.Sprintf("00000000-0000-0000-0001-%012d", i), "org_id": otherOrg, "name": "Shared", "member_projects": []string{}}
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"objects": groups})
			return
		}
		if query.Get("starting_after") != "00000000-0000-0000-0001-000000000099" {
			t.Errorf("unexpected cursor: %s", r.URL.String())
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"objects": []map[string]any{{"id": projectGroupFixtureID, "org_id": targetOrg, "name": "Shared", "member_projects": []string{}}}})
	}))
	defer server.Close()

	c, err := client.NewClient(server.URL, "test-key", "test")
	if err != nil {
		t.Fatal(err)
	}
	groups, err := listProjectGroups(context.Background(), c, targetOrg, "Shared")
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 1 || groups[0].Id.String() != projectGroupFixtureID || requests != 2 {
		t.Fatalf("expected target on second page; got groups=%#v requests=%d", groups, requests)
	}
}
