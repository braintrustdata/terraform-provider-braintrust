package cmd

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/braintrustdata/terraform-provider-braintrust/internal/terraforming/api"
	"github.com/braintrustdata/terraform-provider-braintrust/internal/terraforming/hcl"
)

func TestVerifyACLTargetsKeepsOrderAndSkipsOnlyMissingTargets(t *testing.T) {
	acls := make([]api.ACL, 8)
	for i := range acls {
		acls[i].ID = fmt.Sprintf("acl-%d", i)
		acls[i].ObjectType = "project"
		acls[i].ObjectID = fmt.Sprintf("target-%d", i/2)
	}
	progress := 0
	var reads atomic.Int64
	valid, err := verifyACLTargets(context.Background(), acls, func(_ context.Context, id string) error {
		reads.Add(1)
		if id == "acl-3" {
			return &api.APIError{StatusCode: 400, Message: "ACL does not exist"}
		}
		return nil
	}, func(done, total int) {
		if done == total {
			progress = done
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	for i, ok := range valid {
		if ok != (i != 3) {
			t.Fatalf("ACL %d validity = %v", i, ok)
		}
	}
	if progress != 8 || reads.Load() != 8 {
		t.Fatalf("progress = %d and reads = %d, want 8 each", progress, reads.Load())
	}

	want := errors.New("permission denied")
	_, err = verifyACLTargets(context.Background(), acls, func(_ context.Context, id string) error {
		if id == "acl-3" {
			return want
		}
		return nil
	}, nil)
	if !errors.Is(err, want) {
		t.Fatalf("unexpected ACL read error: %v", err)
	}
}

func TestFetchProjectResourcesBoundsRequestsAndKeepsOrder(t *testing.T) {
	projects := make([]api.Project, 5)
	for i := range projects {
		projects[i].ID = fmt.Sprintf("project-%d", i)
	}
	started := make(chan string, len(projects))
	release := make(chan struct{})
	type result struct {
		items [][]string
		err   error
	}
	finished := make(chan result, 1)
	go func() {
		items, err := fetchProjectResources(context.Background(), projects, "fixtures", func(_ context.Context, id string) ([]string, error) {
			started <- id
			<-release
			return []string{id}, nil
		})
		finished <- result{items, err}
	}()
	for i := 0; i < 4; i++ {
		select {
		case <-started:
		case <-time.After(5 * time.Second):
			close(release)
			t.Fatal("four concurrent reads never started")
		}
	}
	select {
	case id := <-started:
		close(release)
		t.Fatalf("fifth read %s exceeded concurrency limit", id)
	default:
	}
	close(release)
	select {
	case got := <-finished:
		if got.err != nil {
			t.Fatal(got.err)
		}
		for i, items := range got.items {
			if len(items) != 1 || items[0] != projects[i].ID {
				t.Fatalf("project %d has unordered results %v", i, items)
			}
		}
	case <-time.After(5 * time.Second):
		t.Fatal("project reads did not finish")
	}
}

func TestProjectGroupMembershipNameUsesFullIDs(t *testing.T) {
	group := "00000000-0000-4000-8000-000000000001"
	first := projectGroupMembershipName("Shared", group, "00000000-0000-4000-8000-000000000002")
	second := projectGroupMembershipName("Shared", group, "00000000-0000-4000-8000-000000000003")
	otherGroup := projectGroupMembershipName("Shared", "00000000-0000-4000-8000-000000000004", "00000000-0000-4000-8000-000000000002")
	if first == second || first == otherGroup {
		t.Fatalf("membership names collided: %q, %q, %q", first, second, otherGroup)
	}
	if first != projectGroupMembershipName("Shared", group, "00000000-0000-4000-8000-000000000002") {
		t.Fatal("membership name is unstable")
	}
}

func TestRoleBlocksUseOrgIDToDistinguishSystemRoles(t *testing.T) {
	orgID := "selected-org"
	otherOrgID := "other-org"
	roles := []api.Role{
		{ID: "system-1", Name: "New System Role"},
		{ID: "custom-1", OrgID: &orgID, Name: "Viewer"},
		{ID: "other-1", OrgID: &otherOrgID, Name: "Other Org Role"},
	}
	resolver := hcl.NewReferenceResolver([]string{"braintrust_role"})
	blocks := roleBlocksFromRoles(roles, orgID, resolver)
	if len(blocks) != 1 || blocks[0].ID != "custom-1" {
		t.Fatalf("expected only the selected organization's custom role, got %#v", blocks)
	}
	dataSources := resolver.RoleDataSources()
	if len(dataSources) != 1 || dataSources[0].Fields["name"] != "New System Role" {
		t.Fatalf("expected the unknown system role as a data source, got %#v", dataSources)
	}
	if got := resolver.Resolve("role_id", "system-1"); got == "" {
		t.Fatal("system role reference was not registered")
	}
	if got := resolver.Resolve("role_id", "other-1"); got != "" {
		t.Fatalf("cross-org role unexpectedly resolved: %s", got)
	}
}

func TestDefaultBuiltinACLClassification(t *testing.T) {
	const orgID = "org"
	resolver := hcl.NewReferenceResolver(nil)
	resolver.RegisterGroupDataSource("owners", "Owners", orgID)
	resolver.RegisterGroupDataSource("engineers", "Engineers", orgID)
	resolver.RegisterGroupDataSource("viewers", "Viewers", orgID)
	resolver.RegisterGroupDataSource("ai", "All AI Provider Access", orgID)
	resolver.RegisterRoleDataSource("owner-role", "Owner")
	resolver.RegisterRoleDataSource("engineer-role", "Engineer")
	resolver.RegisterRoleDataSource("viewer-role", "Viewer")
	read := "read"
	tests := []struct {
		name string
		acl  api.ACL
		want bool
	}{
		{"owner default", api.ACL{ObjectType: "organization", ObjectID: orgID, GroupID: ptr("owners"), RoleID: ptr("owner-role")}, true},
		{"engineer default", api.ACL{ObjectType: "org_project", ObjectID: orgID, GroupID: ptr("engineers"), RoleID: ptr("engineer-role")}, true},
		{"viewer default", api.ACL{ObjectType: "org_project", ObjectID: orgID, GroupID: ptr("viewers"), RoleID: ptr("viewer-role")}, true},
		{"ai default", api.ACL{ObjectType: "org_ai_secret", ObjectID: orgID, GroupID: ptr("ai"), Permission: &read}, true},
		{"custom project grant", api.ACL{ObjectType: "project", ObjectID: "project", GroupID: ptr("ai"), Permission: &read}, false},
		{"custom role grant", api.ACL{ObjectType: "organization", ObjectID: orgID, GroupID: ptr("owners"), RoleID: ptr("viewer-role")}, false},
		{"different organization", api.ACL{ObjectType: "org_ai_secret", ObjectID: "other-org", GroupID: ptr("ai"), Permission: &read}, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := isDefaultBuiltinACL(test.acl, orgID, resolver); got != test.want {
				t.Fatalf("isDefaultBuiltinACL() = %v, want %v", got, test.want)
			}
		})
	}
}

func ptr(value string) *string { return &value }
