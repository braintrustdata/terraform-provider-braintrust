package cmd

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/braintrustdata/braintrust-sdk-go/openapi"
	"github.com/braintrustdata/terraform-provider-braintrust/internal/client"
	"github.com/braintrustdata/terraform-provider-braintrust/internal/terraforming/hcl"
	"github.com/google/uuid"
	"github.com/oapi-codegen/nullable"
)

// fixtureID returns a stable UUID for test fixture n.
func fixtureID(n int) uuid.UUID {
	return uuid.MustParse(fmt.Sprintf("00000000-0000-0000-0000-%012d", n))
}

func TestVerifyACLTargetsKeepsOrderAndSkipsOnlyMissingTargets(t *testing.T) {
	acls := make([]openapi.Acl, 8)
	for i := range acls {
		acls[i].Id = fixtureID(i)
		acls[i].ObjectType = "project"
		acls[i].ObjectId = fixtureID(100 + i/2)
	}
	missing := fixtureID(3).String()
	progress := 0
	var reads atomic.Int64
	valid, err := verifyACLTargets(context.Background(), acls, func(_ context.Context, id string) error {
		reads.Add(1)
		if id == missing {
			return &client.APIError{StatusCode: 400, Message: "ACL does not exist"}
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

	valid, err = verifyACLTargets(context.Background(), acls, func(_ context.Context, id string) error {
		if id == missing {
			return &client.APIError{StatusCode: 403, Message: "Object Does not exist"}
		}
		return nil
	}, nil)
	if err != nil || valid[3] {
		t.Fatalf("a missing target must be skipped regardless of message case; valid=%v err=%v", valid[3], err)
	}

	want := errors.New("permission denied")
	_, err = verifyACLTargets(context.Background(), acls, func(_ context.Context, id string) error {
		if id == missing {
			return want
		}
		return nil
	}, nil)
	if !errors.Is(err, want) {
		t.Fatalf("unexpected ACL read error: %v", err)
	}
}

func TestFetchProjectResourcesBoundsRequestsAndKeepsOrder(t *testing.T) {
	projects := make([]openapi.Project, 5)
	for i := range projects {
		projects[i].Id = fixtureID(i)
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
			if len(items) != 1 || items[0] != projects[i].Id.String() {
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
	orgID, otherOrgID := fixtureID(10), fixtureID(11)
	system, custom, other := fixtureID(1), fixtureID(2), fixtureID(3)
	roles := []openapi.Role{
		{Id: system, Name: "New System Role"},
		{Id: custom, OrgId: nullable.NewNullableWithValue(orgID), Name: "Viewer"},
		{Id: other, OrgId: nullable.NewNullableWithValue(otherOrgID), Name: "Other Org Role"},
	}
	resolver := hcl.NewReferenceResolver([]string{"braintrust_role"})
	blocks := roleBlocksFromRoles(roles, orgID.String(), resolver)
	if len(blocks) != 1 || blocks[0].ID != custom.String() {
		t.Fatalf("expected only the selected organization's custom role, got %#v", blocks)
	}
	dataSources := resolver.RoleDataSources()
	if len(dataSources) != 1 || dataSources[0].Fields["name"] != "New System Role" {
		t.Fatalf("expected the unknown system role as a data source, got %#v", dataSources)
	}
	if got := resolver.Resolve("role_id", system.String()); got == "" {
		t.Fatal("system role reference was not registered")
	}
	if got := resolver.Resolve("role_id", other.String()); got != "" {
		t.Fatalf("cross-org role unexpectedly resolved: %s", got)
	}
}

func TestDefaultBuiltinACLClassification(t *testing.T) {
	org := fixtureID(1)
	orgID := org.String()
	owners, engineers, viewers, ai := fixtureID(2), fixtureID(3), fixtureID(4), fixtureID(5)
	ownerRole, engineerRole, viewerRole := fixtureID(6), fixtureID(7), fixtureID(8)
	resolver := hcl.NewReferenceResolver(nil)
	resolver.RegisterGroupDataSource(owners.String(), "Owners", orgID)
	resolver.RegisterGroupDataSource(engineers.String(), "Engineers", orgID)
	resolver.RegisterGroupDataSource(viewers.String(), "Viewers", orgID)
	resolver.RegisterGroupDataSource(ai.String(), "All AI Provider Access", orgID)
	resolver.RegisterRoleDataSource(ownerRole.String(), "Owner")
	resolver.RegisterRoleDataSource(engineerRole.String(), "Engineer")
	resolver.RegisterRoleDataSource(viewerRole.String(), "Viewer")
	read := openapi.Permission("read")
	acl := func(objectType openapi.AclObjectType, objectID, group uuid.UUID, role *uuid.UUID, permission *openapi.Permission) openapi.Acl {
		a := openapi.Acl{ObjectType: objectType, ObjectId: objectID, GroupId: nullable.NewNullableWithValue(group), Permission: permission}
		if role != nil {
			a.RoleId = nullable.NewNullableWithValue(*role)
		}
		return a
	}
	tests := []struct {
		name string
		acl  openapi.Acl
		want bool
	}{
		{"owner default", acl("organization", org, owners, &ownerRole, nil), true},
		{"engineer default", acl("org_project", org, engineers, &engineerRole, nil), true},
		{"viewer default", acl("org_project", org, viewers, &viewerRole, nil), true},
		{"ai default", acl("org_ai_secret", org, ai, nil, &read), true},
		{"custom project grant", acl("project", fixtureID(9), ai, nil, &read), false},
		{"custom role grant", acl("organization", org, owners, &viewerRole, nil), false},
		{"different organization", acl("org_ai_secret", fixtureID(10), ai, nil, &read), false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := isDefaultBuiltinACL(test.acl, orgID, resolver); got != test.want {
				t.Fatalf("isDefaultBuiltinACL() = %v, want %v", got, test.want)
			}
		})
	}
}
