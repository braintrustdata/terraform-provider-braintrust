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

func TestProjectGroupMembershipPatchBatches(t *testing.T) {
	var batches []client.UpdateProjectGroupRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodPatch || req.URL.Path != "/v1/project_group/group-id" {
			t.Errorf("unexpected request: %s %s", req.Method, req.URL.Path)
			http.Error(w, "unexpected request", http.StatusBadRequest)
			return
		}
		var batch client.UpdateProjectGroupRequest
		if err := json.NewDecoder(req.Body).Decode(&batch); err != nil {
			t.Errorf("decode: %v", err)
		}
		batches = append(batches, batch)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":"group-id","org_id":"org-id","name":"renamed","member_projects":[]}`)
	}))
	defer server.Close()
	c, err := client.NewClient(server.URL, "test-key", "")
	if err != nil {
		t.Fatal(err)
	}
	adds := make([]string, 1500)
	removes := make([]string, 600)
	for i := range adds {
		adds[i] = fmt.Sprintf("add-%d", i)
	}
	for i := range removes {
		removes[i] = fmt.Sprintf("remove-%d", i)
	}
	name := "renamed"
	value := json.RawMessage(`"new description"`)
	r := &projectGroupResource{client: c}
	_, err = r.applyChanges(context.Background(), "group-id", client.UpdateProjectGroupRequest{
		Name: &name, Description: &value, AddMemberProjects: adds, RemoveMemberProjects: removes,
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
