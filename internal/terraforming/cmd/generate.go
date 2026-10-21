package cmd

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"sync/atomic"

	"github.com/braintrustdata/terraform-provider-braintrust/internal/terraforming/api"
	"github.com/braintrustdata/terraform-provider-braintrust/internal/terraforming/hcl"
	"github.com/braintrustdata/terraform-provider-braintrust/internal/terraforming/schema"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"golang.org/x/sync/errgroup"
)

var generateCmd = &cobra.Command{
	Use:   "generate",
	Short: "Generate Terraform resource configuration from existing Braintrust resources",
	Long: `Fetches resources from the Braintrust API and generates the corresponding
Terraform resource blocks (HCL). Output can be piped to a .tf file.`,
	RunE: runGenerate,
}

func init() {
	rootCmd.AddCommand(generateCmd)
}

func runGenerate(cmd *cobra.Command, args []string) error {
	resourceTypes, err := resolveResourceTypes()
	if err != nil {
		return err
	}

	registry, err := schema.LoadRegistry()
	if err != nil {
		return fmt.Errorf("failed to load schema registry: %w", err)
	}

	ctx := context.Background()
	blocks, vars, dataSources, err := fetchAndBuildBlocks(ctx, resourceTypes)
	if err != nil {
		return err
	}

	writer := hcl.NewWriter(registry)
	output, shouldClose, err := getOutput()
	if err != nil {
		return err
	}
	if shouldClose {
		defer output.Close()
	}

	// Write user data sources first so resources can reference them.
	if len(dataSources) > 0 {
		dataSourceHCL := writer.WriteDataSources(dataSources)
		if _, err := output.Write(dataSourceHCL); err != nil {
			return fmt.Errorf("failed to write data sources: %w", err)
		}
		if _, err := output.Write([]byte("\n")); err != nil {
			return fmt.Errorf("failed to write separator: %w", err)
		}
	}

	data := writer.WriteResources(blocks)
	if _, err := output.Write(data); err != nil {
		return fmt.Errorf("failed to write output: %w", err)
	}

	// Write variable blocks for secrets.
	if len(vars) > 0 {
		varData := writer.WriteVariables(vars)
		if _, err := output.Write(varData); err != nil {
			return fmt.Errorf("failed to write variables: %w", err)
		}
		fmt.Fprintf(os.Stderr, "Generated %d required sensitive variable(s) for write-only values; supply them before applying (for example, with TF_VAR_<variable_name>). See variable blocks in the output.\n", len(vars))
	}

	if shouldClose {
		fmt.Fprintf(os.Stderr, "Generated %d resource(s)\n", len(blocks))
	}

	return nil
}

// fetchAndBuildBlocks fetches resources from the API and builds HCL blocks.
func fetchAndBuildBlocks(ctx context.Context, resourceTypes []string) ([]hcl.ResourceBlock, []hcl.VariableBlock, []hcl.DataBlock, error) {
	linkedTypes := viper.GetStringSlice("linked-resource-types")
	if viper.GetBool("no-linked-resources") {
		linkedTypes = nil
	} else if len(linkedTypes) == 0 {
		linkedTypes = DefaultLinkedResourceTypes
	}
	resolver := hcl.NewReferenceResolver(linkedTypes)

	projectFilter := viper.GetStringSlice("projects")
	explicitOrgID := strings.TrimSpace(viper.GetString("org-id"))

	// Resolve the org before any org-wide discovery. A project filter must not
	// silently choose an org for a multi-org credential.
	visibleProjects, err := apiClient.ListProjects(ctx)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to list projects: %w", err)
	}
	orgID, err := resolveOrgID(ctx, visibleProjects, explicitOrgID)
	if err != nil {
		return nil, nil, nil, err
	}
	var orgProjects []api.Project
	for _, p := range visibleProjects {
		if p.OrgID == orgID {
			orgProjects = append(orgProjects, p)
		}
	}
	projects, err := filterProjects(orgProjects, projectFilter)
	if err != nil {
		return nil, nil, nil, err
	}
	fmt.Fprintf(os.Stderr, "Selected organization %s and %d project(s)\n", orgID, len(projects))

	// Link only to projects that will actually be emitted.
	if contains(resourceTypes, "braintrust_project") {
		for _, p := range projects {
			name := hcl.ResourceNameFromIDAndLabel(p.ID, p.Name)
			resolver.Register(p.ID, "braintrust_project", name)
		}
	}

	// Phase 2: Fetch users for user-ID linking (groups and ACLs).
	var users []api.User
	if (resolver.HasLinkedTypes() && (contains(resourceTypes, "braintrust_group") || contains(resourceTypes, "braintrust_acl"))) || contains(resourceTypes, "braintrust_group_membership") {
		var err error
		users, err = apiClient.ListUsers(ctx)
		if err != nil {
			if contains(resourceTypes, "braintrust_group_membership") {
				return nil, nil, nil, fmt.Errorf("failed to list users for built-in group membership: %w", err)
			}
			fmt.Fprintf(os.Stderr, "Warning: failed to list users for linking: %v\n", err)
		} else {
			for _, u := range users {
				if u.Email != nil {
					resolver.RegisterUser(u.ID, *u.Email)
				}
			}
		}
	}

	// Phase 3: Fetch all requested resource types.
	var blocks []hcl.ResourceBlock
	var vars []hcl.VariableBlock
	requestedSet := make(map[string]bool)
	for _, t := range resourceTypes {
		requestedSet[t] = true
	}

	// Fetch in dependency order.
	orderedTypes := []string{
		"braintrust_project",
		"braintrust_role",
		"braintrust_group",
		"braintrust_group_membership",
		"braintrust_project_group",
		"braintrust_project_group_membership",
		"braintrust_acl",
		"braintrust_dataset",
		"braintrust_function",
		"braintrust_project_score",
		"braintrust_project_tag",
		"braintrust_project_automation",
		"braintrust_org_automation",
		"braintrust_mcp_server",
		"braintrust_env_var",
		"braintrust_service_token",
		"braintrust_ai_secret",
		"braintrust_view",
	}

	for _, resourceType := range orderedTypes {
		if !requestedSet[resourceType] {
			continue
		}

		var newBlocks []hcl.ResourceBlock
		var newVars []hcl.VariableBlock
		var err error

		switch resourceType {
		case "braintrust_project":
			newBlocks, err = buildProjectBlocks(projects, resolver)
		case "braintrust_role":
			newBlocks, err = buildRoleBlocks(ctx, orgID, resolver)
		case "braintrust_group":
			newBlocks, err = buildGroupBlocks(ctx, orgID, resolver)
		case "braintrust_group_membership":
			newBlocks, err = buildBuiltinGroupMembershipBlocks(ctx, orgID, resolver)
		case "braintrust_project_group":
			newBlocks, err = buildProjectGroupBlocks(ctx, orgID, resolver, !requestedSet["braintrust_project_group_membership"])
		case "braintrust_project_group_membership":
			newBlocks, err = buildProjectGroupMembershipBlocks(ctx, orgID, resolver)
		case "braintrust_acl":
			newBlocks, err = buildACLBlocks(ctx, orgID, resolver)
		case "braintrust_dataset":
			newBlocks, err = buildDatasetBlocks(ctx, projects, resolver)
		case "braintrust_function":
			newBlocks, err = buildFunctionBlocks(ctx, projects, resolver)
		case "braintrust_project_score":
			newBlocks, err = buildProjectScoreBlocks(ctx, projects, resolver)
		case "braintrust_project_tag":
			newBlocks, err = buildProjectTagBlocks(ctx, projects, resolver)
		case "braintrust_project_automation":
			newBlocks, err = buildProjectAutomationBlocks(ctx, projects, resolver)
		case "braintrust_org_automation":
			newBlocks, err = buildOrgAutomationBlocks(ctx, orgID)
		case "braintrust_mcp_server":
			newBlocks, err = buildMCPServerBlocks(ctx, projects, resolver)
		case "braintrust_env_var":
			newBlocks, newVars, err = buildEnvVarBlocks(ctx, projects, orgID, resolver)
		case "braintrust_service_token":
			newBlocks, err = buildServiceTokenBlocks(ctx, orgID, resolver)
		case "braintrust_ai_secret":
			newBlocks, newVars, err = buildAISecretBlocks(ctx, orgID, resolver)
		case "braintrust_view":
			newBlocks, err = buildViewBlocks(ctx, projects, orgID, resolver)
		}

		if err != nil {
			return nil, nil, nil, fmt.Errorf("failed to fetch %s: %w", resourceType, err)
		}

		fmt.Fprintf(os.Stderr, "  %s: %d resource(s)\n", resourceType, len(newBlocks))
		blocks = append(blocks, newBlocks...)
		vars = append(vars, newVars...)
	}

	dataSources := append(resolver.UserDataSources(), resolver.RoleDataSources()...)
	dataSources = append(dataSources, resolver.GroupDataSources()...)
	return blocks, vars, dataSources, nil
}

// =============================================================================
// Block builders for each resource type.
// =============================================================================

func buildProjectBlocks(projects []api.Project, resolver *hcl.ReferenceResolver) ([]hcl.ResourceBlock, error) {
	var blocks []hcl.ResourceBlock
	for _, p := range projects {
		name := hcl.ResourceNameFromIDAndLabel(p.ID, p.Name)
		fields := map[string]any{"name": p.Name}
		if p.Description != nil {
			fields["description"] = *p.Description
		}
		blocks = append(blocks, hcl.ResourceBlock{
			Type:    "braintrust_project",
			Name:    name,
			Comment: p.Name,
			ID:      p.ID,
			Fields:  fields,
		})
	}
	return blocks, nil
}

func buildRoleBlocks(ctx context.Context, orgID string, resolver *hcl.ReferenceResolver) ([]hcl.ResourceBlock, error) {
	roles, err := apiClient.ListRoles(ctx)
	if err != nil {
		return nil, err
	}
	return roleBlocksFromRoles(roles, orgID, resolver), nil
}

func roleBlocksFromRoles(roles []api.Role, orgID string, resolver *hcl.ReferenceResolver) []hcl.ResourceBlock {
	var blocks []hcl.ResourceBlock
	for _, r := range roles {
		if r.OrgID != nil && *r.OrgID != orgID {
			continue
		}
		// Null org_id is the API contract for a system role. System roles
		// cannot be managed, but ACLs and custom roles may reference them.
		if r.OrgID == nil {
			resolver.RegisterRoleDataSource(r.ID, r.Name)
			continue
		}

		name := hcl.ResourceNameFromIDAndLabel(r.ID, r.Name)
		resolver.Register(r.ID, "braintrust_role", name)

		fields := map[string]any{"name": r.Name}
		if r.Description != nil {
			fields["description"] = *r.Description
		}
		if len(r.MemberPermissions) > 0 {
			perms := make([]any, len(r.MemberPermissions))
			for i, mp := range r.MemberPermissions {
				pm := map[string]any{"permission": mp.Permission}
				if mp.RestrictObjectType != nil {
					pm["restrict_object_type"] = *mp.RestrictObjectType
				}
				perms[i] = pm
			}
			fields["member_permissions"] = perms
		}
		if len(r.MemberRoles) > 0 {
			fields["member_roles"] = r.MemberRoles
		}

		blocks = append(blocks, hcl.ResourceBlock{
			Type:    "braintrust_role",
			Name:    name,
			Comment: r.Name,
			ID:      r.ID,
			Fields:  fields,
		})
	}
	return blocks
}

func buildGroupBlocks(ctx context.Context, orgID string, resolver *hcl.ReferenceResolver) ([]hcl.ResourceBlock, error) {
	groups, err := apiClient.ListGroups(ctx)
	if err != nil {
		return nil, err
	}

	unresolvedMemberIDs := make(map[string]struct{})
	linkUsers := resolver.HasLinkedTypes()
	strictUserLinking := viper.GetBool("strict-user-linking")

	var blocks []hcl.ResourceBlock
	for _, g := range groups {
		if g.OrgID != orgID {
			continue
		}
		if isBuiltinGroup(g.Name) {
			resolver.RegisterGroupDataSource(g.ID, g.Name, g.OrgID)
			continue
		}
		name := hcl.ResourceNameFromIDAndLabel(g.ID, g.Name)
		resolver.Register(g.ID, "braintrust_group", name)

		fields := map[string]any{"name": g.Name}
		if g.Description != nil {
			fields["description"] = *g.Description
		}
		if len(g.MemberUsers) > 0 {
			memberUsers := make([]any, 0, len(g.MemberUsers))
			for _, userID := range g.MemberUsers {
				if !linkUsers {
					memberUsers = append(memberUsers, userID)
					continue
				}
				if ref, ok := resolver.UserRef(userID); ok {
					memberUsers = append(memberUsers, hcl.RawExpression(ref))
				} else {
					if strictUserLinking {
						return nil, fmt.Errorf("strict user linking enabled: group member %s in group %q was not returned with an email by GET /v1/user (the endpoint excludes service accounts)", userID, g.Name)
					}
					unresolvedMemberIDs[userID] = struct{}{}
					memberUsers = append(memberUsers, userID)
				}
			}
			fields["member_users"] = memberUsers
		}
		if len(g.MemberGroups) > 0 {
			fields["member_groups"] = g.MemberGroups
		}

		blocks = append(blocks, hcl.ResourceBlock{
			Type:    "braintrust_group",
			Name:    name,
			Comment: g.Name,
			ID:      g.ID,
			Fields:  fields,
		})
	}
	if len(unresolvedMemberIDs) > 0 {
		fmt.Fprintf(os.Stderr, "Warning: %d distinct group member ID(s) were not returned with an email by GET /v1/user; emitting raw IDs in member_users. This endpoint excludes service accounts; missing IDs may also be stale or inaccessible. Review these IDs before using the export in another organization.\n", len(unresolvedMemberIDs))
	}
	return blocks, nil
}

func isBuiltinGroup(name string) bool {
	switch name {
	case "Owners", "Engineers", "Viewers", "All AI Provider Access":
		return true
	default:
		return false
	}
}

func buildBuiltinGroupMembershipBlocks(ctx context.Context, orgID string, resolver *hcl.ReferenceResolver) ([]hcl.ResourceBlock, error) {
	groups, err := apiClient.ListGroups(ctx)
	if err != nil {
		return nil, err
	}
	var blocks []hcl.ResourceBlock
	skippedIDs := make(map[string]struct{})
	for _, group := range groups {
		if group.OrgID != orgID || !isBuiltinGroup(group.Name) {
			continue
		}
		resolver.RegisterGroupDataSource(group.ID, group.Name, group.OrgID)
		for _, userID := range group.MemberUsers {
			// GET /v1/user only exposes human users. The API does not say
			// whether a service-account membership is manual or automatic,
			// so never put an unresolved account under Terraform ownership.
			if resolver.UserEmail(userID) == "" {
				skippedIDs[userID] = struct{}{}
				continue
			}
			fields := map[string]any{"group_id": group.ID, "user_id": userID}
			linked := map[string]string{}
			if ref := resolver.Resolve("group_id", group.ID); ref != "" {
				linked["group_id"] = ref
			}
			if resolver.HasLinkedTypes() {
				if ref, ok := resolver.UserRef(userID); ok {
					linked["user_id"] = ref
				}
			}
			sum := sha256.Sum256([]byte(group.ID + "/" + userID))
			blocks = append(blocks, hcl.ResourceBlock{
				Type:    "braintrust_group_membership",
				Name:    fmt.Sprintf("member_%x", sum[:12]),
				Comment: group.Name + " — " + resolver.UserEmail(userID),
				ID:      group.ID + "/" + userID,
				Fields:  fields, LinkedFields: linked,
			})
		}
	}
	if len(skippedIDs) > 0 {
		fmt.Fprintf(os.Stderr, "Warning: skipped %d distinct built-in group member ID(s) absent from GET /v1/user. These may include Braintrust-managed service accounts; their membership remains unmanaged.\n", len(skippedIDs))
	}
	return blocks, nil
}

func buildProjectGroupBlocks(ctx context.Context, orgID string, resolver *hcl.ReferenceResolver, inlineMembers bool) ([]hcl.ResourceBlock, error) {
	groups, err := apiClient.ListProjectGroups(ctx, orgID)
	if err != nil {
		return nil, err
	}
	var blocks []hcl.ResourceBlock
	for _, group := range groups {
		if group.OrgID != orgID {
			return nil, fmt.Errorf("project group %s belongs to unexpected organization %s", group.ID, group.OrgID)
		}
		name := hcl.ResourceNameFromIDAndLabel(group.ID, group.Name)
		resolver.Register(group.ID, "braintrust_project_group", name)
		fields := map[string]any{"org_id": group.OrgID, "name": group.Name}
		if group.Description != nil {
			fields["description"] = *group.Description
		}
		if inlineMembers && len(group.MemberProjects) > 0 {
			members := make([]any, 0, len(group.MemberProjects))
			for _, projectID := range group.MemberProjects {
				if ref := resolver.Resolve("member_projects", projectID); ref != "" {
					members = append(members, hcl.RawExpression(ref))
				} else {
					members = append(members, projectID)
				}
			}
			fields["member_projects"] = members
		}
		blocks = append(blocks, hcl.ResourceBlock{Type: "braintrust_project_group", Name: name, Comment: group.Name, ID: group.ID, Fields: fields})
	}
	return blocks, nil
}

func buildProjectGroupMembershipBlocks(ctx context.Context, orgID string, resolver *hcl.ReferenceResolver) ([]hcl.ResourceBlock, error) {
	groups, err := apiClient.ListProjectGroups(ctx, orgID)
	if err != nil {
		return nil, err
	}
	var blocks []hcl.ResourceBlock
	for _, group := range groups {
		if group.OrgID != orgID {
			return nil, fmt.Errorf("project group %s belongs to unexpected organization %s", group.ID, group.OrgID)
		}
		for _, projectID := range group.MemberProjects {
			fields := map[string]any{"project_group_id": group.ID, "project_id": projectID}
			linked := map[string]string{}
			if ref := resolver.Resolve("project_group_id", group.ID); ref != "" {
				linked["project_group_id"] = ref
			}
			if ref := resolver.Resolve("project_id", projectID); ref != "" {
				linked["project_id"] = ref
			}
			blocks = append(blocks, hcl.ResourceBlock{
				Type:   "braintrust_project_group_membership",
				Name:   projectGroupMembershipName(group.Name, group.ID, projectID),
				ID:     group.ID + "/" + projectID,
				Fields: fields, LinkedFields: linked,
			})
		}
	}
	return blocks, nil
}

func projectGroupMembershipName(groupName, groupID, projectID string) string {
	label := api.SanitizeName(groupName)
	if label == "" {
		label = "membership"
	}
	sum := sha256.Sum256([]byte(groupID + "/" + projectID))
	return fmt.Sprintf("%s_%x", label, sum[:12])
}

func buildACLBlocks(ctx context.Context, orgID string, resolver *hcl.ReferenceResolver) ([]hcl.ResourceBlock, error) {
	var blocks []hcl.ResourceBlock
	// Discover the system principals even when only ACLs were requested, so
	// default Braintrust grants are never imported as managed ACL resources.
	groups, err := apiClient.ListGroups(ctx)
	if err != nil {
		return nil, fmt.Errorf("list groups for ACL classification: %w", err)
	}
	for _, group := range groups {
		if group.OrgID == orgID && isBuiltinGroup(group.Name) {
			resolver.RegisterGroupDataSource(group.ID, group.Name, group.OrgID)
		}
	}
	roles, err := apiClient.ListRoles(ctx)
	if err != nil {
		return nil, fmt.Errorf("list roles for ACL classification: %w", err)
	}
	for _, role := range roles {
		if role.OrgID == nil {
			resolver.RegisterRoleDataSource(role.ID, role.Name)
		}
	}
	fmt.Fprintln(os.Stderr, "  braintrust_acl: discovering organization ACLs...")
	acls, err := apiClient.ListOrgACLs(ctx)
	if err != nil {
		return nil, fmt.Errorf("list org ACLs: %w", err)
	}
	var candidates []api.ACL
	for _, a := range acls {
		if a.ObjectOrgID != orgID {
			return nil, fmt.Errorf("ACL %s belongs to organization %q, not selected organization %q; use a credential scoped to the selected org", a.ID, a.ObjectOrgID, orgID)
		}
		if isDefaultBuiltinACL(a, orgID, resolver) {
			continue
		}
		candidates = append(candidates, a)
	}
	var valid []bool
	if viper.GetBool("verify-acl-targets") {
		if len(candidates) > 0 {
			fmt.Fprintf(os.Stderr, "  braintrust_acl: verifying %d ACL(s), up to 4 concurrent reads...\n", len(candidates))
		}
		valid, err = verifyACLTargets(ctx, candidates, func(ctx context.Context, id string) error {
			_, err := apiClient.GetACL(ctx, id)
			return err
		}, func(done, total int) {
			fmt.Fprintf(os.Stderr, "  braintrust_acl: verified %d/%d ACL(s)\n", done, total)
		})
		if err != nil {
			return nil, err
		}
	} else {
		valid = make([]bool, len(candidates))
		for i := range valid {
			valid[i] = true
		}
		if len(candidates) > 0 {
			targets := make(map[[2]string]struct{})
			for _, a := range candidates {
				targets[[2]string{a.ObjectType, a.ObjectID}] = struct{}{}
			}
			fmt.Fprintf(os.Stderr, "  braintrust_acl: %d distinct target scope(s) for %d unverified ACL(s)\n", len(targets), len(candidates))
			fmt.Fprintf(os.Stderr, "Warning: %d ACL target(s) were not verified; GET /v1/acl/list_org can include grants to deleted targets. Review ACL imports before applying, or rerun with --verify-acl-targets.\n", len(candidates))
		}
	}
	stale := 0
	for i, a := range candidates {
		if !valid[i] {
			stale++
			continue
		}

		name := hcl.ResourceNameFromID(a.ID)
		fields := map[string]any{
			"object_type": a.ObjectType,
			"object_id":   a.ObjectID,
		}
		linked := make(map[string]string)

		if a.GroupID != nil {
			if ref := resolver.Resolve("group_id", *a.GroupID); ref != "" {
				linked["group_id"] = ref
			} else {
				fields["group_id"] = *a.GroupID
			}
		}
		if a.RoleID != nil {
			if ref := resolver.Resolve("role_id", *a.RoleID); ref != "" {
				linked["role_id"] = ref
			} else {
				fields["role_id"] = *a.RoleID
			}
		}
		if a.UserID != nil {
			if ref, ok := resolver.UserRef(*a.UserID); ok {
				linked["user_id"] = ref
			} else {
				fields["user_id"] = *a.UserID
			}
		}
		if a.Permission != nil {
			fields["permission"] = *a.Permission
		}
		if a.RestrictObjectType != nil {
			fields["restrict_object_type"] = *a.RestrictObjectType
		}

		// Resolve direct ACL targets that are emitted as managed resources.
		if a.ObjectType == "project" || a.ObjectType == "project_group" {
			if ref := resolver.Resolve("object_id", a.ObjectID); ref != "" {
				linked["object_id"] = ref
			}
		}

		blocks = append(blocks, hcl.ResourceBlock{
			Type:         "braintrust_acl",
			Name:         name,
			ID:           a.ID,
			Fields:       fields,
			LinkedFields: linked,
		})
	}
	if stale > 0 {
		fmt.Fprintf(os.Stderr, "Warning: skipped %d ACL(s) whose individual read reported a missing or inaccessible target\n", stale)
	}

	return blocks, nil
}

// verifyACLTargets checks each ACL independently because the org-wide list can
// include grants whose targets are deleted or inaccessible. Results remain in
// input order for deterministic HCL.
func verifyACLTargets(ctx context.Context, acls []api.ACL, read func(context.Context, string) error, progress func(done, total int)) ([]bool, error) {
	valid := make([]bool, len(acls))
	var completed atomic.Int64
	step := max(25, (len(acls)+9)/10)
	group, readCtx := errgroup.WithContext(ctx)
	group.SetLimit(4)
	for i := range acls {
		group.Go(func() error {
			err := read(readCtx, acls[i].ID)
			if err != nil {
				var apiErr *api.APIError
				if !(errors.As(err, &apiErr) && (apiErr.StatusCode == 400 || apiErr.StatusCode == 403 || apiErr.StatusCode == 404) && strings.Contains(strings.ToLower(apiErr.Message), "does not exist")) {
					return fmt.Errorf("read org ACL %s: %w", acls[i].ID, err)
				}
			} else {
				valid[i] = true
			}
			done := int(completed.Add(1))
			if progress != nil && (done%step == 0 || done == len(acls)) {
				progress(done, len(acls))
			}
			return nil
		})
	}
	if err := group.Wait(); err != nil {
		return nil, err
	}
	return valid, nil
}

func isDefaultBuiltinACL(acl api.ACL, orgID string, resolver *hcl.ReferenceResolver) bool {
	if acl.GroupID == nil || acl.ObjectID != orgID || acl.UserID != nil || acl.RestrictObjectType != nil {
		return false
	}
	groupName := resolver.BuiltinGroupName(*acl.GroupID)
	if groupName == "" {
		return false
	}
	if acl.ObjectType == "org_ai_secret" && acl.RoleID == nil && acl.Permission != nil && *acl.Permission == "read" {
		return groupName == "Engineers" || groupName == "Viewers" || groupName == "All AI Provider Access"
	}
	if acl.RoleID == nil || acl.Permission != nil {
		return false
	}
	roleName := resolver.SystemRoleName(*acl.RoleID)
	switch groupName {
	case "Owners":
		return acl.ObjectType == "organization" && roleName == "Owner"
	case "Engineers":
		return acl.ObjectType == "org_project" && roleName == "Engineer"
	case "Viewers":
		return acl.ObjectType == "org_project" && roleName == "Viewer"
	}
	return false
}

// groupByProject keeps API order within each project while allowing one paginated
// list request per resource type. Callers only emit entries for selected projects.
func groupByProject[T any](items []T, projectID func(T) string) map[string][]T {
	grouped := make(map[string][]T)
	for _, item := range items {
		id := projectID(item)
		grouped[id] = append(grouped[id], item)
	}
	return grouped
}

// fetchProjectResources only handles API reads; callers build Terraform blocks
// afterward in project order so references and output remain deterministic.
func fetchProjectResources[T any](ctx context.Context, projects []api.Project, kind string, fetch func(context.Context, string) ([]T, error)) ([][]T, error) {
	results := make([][]T, len(projects))
	group, fetchCtx := errgroup.WithContext(ctx)
	group.SetLimit(4)
	for i, project := range projects {
		group.Go(func() error {
			items, err := fetch(fetchCtx, project.ID)
			if err != nil {
				return fmt.Errorf("list %s for project %s: %w", kind, project.ID, err)
			}
			results[i] = items
			return nil
		})
	}
	if err := group.Wait(); err != nil {
		return nil, err
	}
	return results, nil
}

func buildDatasetBlocks(ctx context.Context, projects []api.Project, resolver *hcl.ReferenceResolver) ([]hcl.ResourceBlock, error) {
	datasets, err := apiClient.ListDatasets(ctx, "")
	if err != nil {
		return nil, err
	}
	byProject := groupByProject(datasets, func(dataset api.Dataset) string { return dataset.ProjectID })
	var blocks []hcl.ResourceBlock
	for _, p := range projects {
		for _, d := range byProject[p.ID] {
			name := hcl.ResourceNameFromIDAndLabel(d.ID, d.Name)
			fields := map[string]any{"name": d.Name}
			linked := make(map[string]string)

			if ref := resolver.Resolve("project_id", d.ProjectID); ref != "" {
				linked["project_id"] = ref
			} else {
				fields["project_id"] = d.ProjectID
			}
			if d.Description != nil {
				fields["description"] = *d.Description
			}
			if d.Tags != nil {
				fields["tags"] = d.Tags
			}
			if len(d.Metadata) > 0 {
				var shape any
				if err := json.Unmarshal(d.Metadata, &shape); err != nil {
					return nil, fmt.Errorf("dataset %s has invalid metadata: %w", d.ID, err)
				}
				if shape != nil {
					if _, ok := shape.(map[string]any); !ok {
						return nil, fmt.Errorf("dataset %s metadata must be an object", d.ID)
					}
					canonical, err := canonicalJSON(d.Metadata)
					if err != nil {
						return nil, fmt.Errorf("dataset %s has invalid metadata: %w", d.ID, err)
					}
					fields["metadata_json"] = canonical
				}
			}

			blocks = append(blocks, hcl.ResourceBlock{
				Type:         "braintrust_dataset",
				Name:         name,
				Comment:      d.Name,
				ID:           d.ID,
				Fields:       fields,
				LinkedFields: linked,
			})
		}
	}
	return blocks, nil
}

func buildProjectScoreBlocks(ctx context.Context, projects []api.Project, resolver *hcl.ReferenceResolver) ([]hcl.ResourceBlock, error) {
	scores, err := apiClient.ListProjectScores(ctx, "")
	if err != nil {
		return nil, err
	}
	byProject := groupByProject(scores, func(score api.ProjectScore) string { return score.ProjectID })
	var blocks []hcl.ResourceBlock
	for _, p := range projects {
		for _, s := range byProject[p.ID] {
			name := hcl.ResourceNameFromIDAndLabel(s.ID, s.Name)
			fields := map[string]any{
				"name":       s.Name,
				"score_type": s.ScoreType,
			}
			linked := make(map[string]string)

			if ref := resolver.Resolve("project_id", s.ProjectID); ref != "" {
				linked["project_id"] = ref
			} else {
				fields["project_id"] = s.ProjectID
			}
			if s.Description != nil {
				fields["description"] = *s.Description
			}
			if len(s.Categories) > 0 {
				var shape any
				if err := json.Unmarshal(s.Categories, &shape); err != nil {
					return nil, fmt.Errorf("score %s has invalid categories: %w", s.ID, err)
				}
				if shape != nil {
					switch shape.(type) {
					case []any, map[string]any:
					default:
						return nil, fmt.Errorf("score %s categories must be an array or object", s.ID)
					}
				}
				categories, err := canonicalJSON(s.Categories)
				if err != nil {
					return nil, fmt.Errorf("score %s has invalid categories: %w", s.ID, err)
				}
				if categories != "null" {
					fields["categories_json"] = categories
				}
			}
			if len(s.Config) > 0 {
				var shape any
				if err := json.Unmarshal(s.Config, &shape); err != nil {
					return nil, fmt.Errorf("score %s has invalid config: %w", s.ID, err)
				}
				if shape != nil {
					if _, ok := shape.(map[string]any); !ok {
						return nil, fmt.Errorf("score %s config must be an object", s.ID)
					}
				}
				config, err := canonicalJSON(s.Config)
				if err != nil {
					return nil, fmt.Errorf("score %s has invalid config: %w", s.ID, err)
				}
				if config != "null" {
					fields["config_json"] = config
				}
			}

			blocks = append(blocks, hcl.ResourceBlock{
				Type:         "braintrust_project_score",
				Name:         name,
				Comment:      s.Name,
				ID:           s.ID,
				Fields:       fields,
				LinkedFields: linked,
			})
		}
	}
	return blocks, nil
}

func buildViewBlocks(ctx context.Context, projects []api.Project, orgID string, resolver *hcl.ReferenceResolver) ([]hcl.ResourceBlock, error) {
	selectedProjects := make(map[string]bool, len(projects))
	for _, p := range projects {
		selectedProjects[p.ID] = true
	}
	var views []api.View
	projectViews, err := fetchProjectResources(ctx, projects, "views", func(ctx context.Context, projectID string) ([]api.View, error) {
		return apiClient.ListViews(ctx, "project", projectID)
	})
	if err != nil {
		return nil, err
	}
	for _, projectViews := range projectViews {
		views = append(views, projectViews...)
	}
	orgViews, err := apiClient.ListViews(ctx, "org_project", orgID)
	if err != nil {
		return nil, fmt.Errorf("list organization project views: %w", err)
	}
	for _, v := range orgViews {
		// An org_project monitor can be associated with one project through options.
		// Respect --projects without dropping organization-wide views.
		var options struct {
			Options struct {
				ProjectID string `json:"projectId"`
			} `json:"options"`
		}
		if len(v.Options) > 0 && string(v.Options) != "null" {
			if err := json.Unmarshal(v.Options, &options); err != nil {
				return nil, fmt.Errorf("view %s has invalid options: %w", v.ID, err)
			}
		}
		if options.Options.ProjectID != "" && !selectedProjects[options.Options.ProjectID] {
			continue
		}
		views = append(views, v)
	}
	blocks := make([]hcl.ResourceBlock, 0, len(views))
	for _, v := range views {
		fields := map[string]any{"object_type": v.ObjectType, "view_type": v.ViewType, "name": v.Name}
		linked := map[string]string{}
		if ref := resolver.Resolve("object_id", v.ObjectID); ref != "" {
			linked["object_id"] = ref
		} else {
			fields["object_id"] = v.ObjectID
		}
		if v.Description != nil {
			fields["description"] = *v.Description
		}
		if v.Starred != nil {
			fields["starred"] = *v.Starred
		}
		for _, rawField := range []struct {
			key string
			raw json.RawMessage
		}{{"view_data_json", v.ViewData}, {"options_json", v.Options}} {
			if len(rawField.raw) == 0 || string(rawField.raw) == "null" {
				continue
			}
			canonical, err := canonicalJSON(rawField.raw)
			if err != nil {
				return nil, fmt.Errorf("view %s has invalid %s: %w", v.ID, rawField.key, err)
			}
			fields[rawField.key] = canonical
			if rawField.key == "options_json" && v.ObjectType == "org_project" {
				var options struct {
					Options struct {
						ProjectID string `json:"projectId"`
					} `json:"options"`
				}
				if err := json.Unmarshal(rawField.raw, &options); err != nil {
					return nil, fmt.Errorf("view %s has invalid options: %w", v.ID, err)
				}
				if ref := resolver.Resolve("project_id", options.Options.ProjectID); ref != "" {
					literal := `"projectId":"` + options.Options.ProjectID + `"`
					expression := `"projectId":` + ref
					fields[rawField.key] = hcl.RawExpression("jsonencode(" + strings.Replace(canonical, literal, expression, 1) + ")")
				}
			}
		}
		blocks = append(blocks, hcl.ResourceBlock{
			Type: "braintrust_view", Name: hcl.ResourceNameFromIDAndLabel(v.ID, v.Name), Comment: v.Name,
			ID: v.ObjectType + "/" + v.ObjectID + "/" + v.ID, Fields: fields, LinkedFields: linked,
		})
	}
	return blocks, nil
}

func buildFunctionBlocks(ctx context.Context, projects []api.Project, resolver *hcl.ReferenceResolver) ([]hcl.ResourceBlock, error) {
	functions, err := apiClient.ListFunctions(ctx, "")
	if err != nil {
		return nil, err
	}
	byProject := groupByProject(functions, func(function api.Function) string { return function.ProjectID })
	var blocks []hcl.ResourceBlock
	for _, project := range projects {
		for _, function := range byProject[project.ID] {
			fields := map[string]any{
				"name": function.Name,
				"slug": function.Slug,
			}
			linked := make(map[string]string)
			if ref := resolver.Resolve("project_id", function.ProjectID); ref != "" {
				linked["project_id"] = ref
			} else {
				fields["project_id"] = function.ProjectID
			}
			if function.Description != nil {
				fields["description"] = *function.Description
			}
			if len(function.Tags) > 0 {
				fields["tags"] = function.Tags
			}
			if function.FunctionType != nil {
				fields["function_type"] = *function.FunctionType
			}
			if len(function.FunctionData) == 0 || string(function.FunctionData) == "null" {
				return nil, fmt.Errorf("function %s is missing function_data", function.ID)
			}
			canonical, err := canonicalJSON(function.FunctionData)
			if err != nil {
				return nil, fmt.Errorf("function %s has invalid function_data: %w", function.ID, err)
			}
			fields["function_data_json"] = canonical
			if len(function.PromptData) > 0 && string(function.PromptData) != "null" {
				canonical, err := canonicalJSON(function.PromptData)
				if err != nil {
					return nil, fmt.Errorf("function %s has invalid prompt_data: %w", function.ID, err)
				}
				fields["prompt_data_json"] = canonical
			}
			if len(function.FunctionSchema) > 0 && string(function.FunctionSchema) != "null" {
				canonical, err := canonicalJSON(function.FunctionSchema)
				if err != nil {
					return nil, fmt.Errorf("function %s has invalid function_schema: %w", function.ID, err)
				}
				fields["function_schema_json"] = canonical
			}
			blocks = append(blocks, hcl.ResourceBlock{
				Type:         "braintrust_function",
				Name:         hcl.ResourceNameFromIDAndLabel(function.ID, function.Name),
				Comment:      function.Name,
				ID:           function.ID,
				Fields:       fields,
				LinkedFields: linked,
			})
		}
	}
	return blocks, nil
}

func buildProjectTagBlocks(ctx context.Context, projects []api.Project, resolver *hcl.ReferenceResolver) ([]hcl.ResourceBlock, error) {
	tags, err := apiClient.ListProjectTags(ctx, "")
	if err != nil {
		return nil, err
	}
	byProject := groupByProject(tags, func(tag api.ProjectTag) string { return tag.ProjectID })
	var blocks []hcl.ResourceBlock
	for _, p := range projects {
		for _, t := range byProject[p.ID] {
			name := hcl.ResourceNameFromIDAndLabel(t.ID, t.Name)
			fields := map[string]any{"name": t.Name}
			linked := make(map[string]string)

			if ref := resolver.Resolve("project_id", t.ProjectID); ref != "" {
				linked["project_id"] = ref
			} else {
				fields["project_id"] = t.ProjectID
			}
			if t.Description != nil {
				fields["description"] = *t.Description
			}
			if t.Color != nil {
				fields["color"] = *t.Color
			}

			blocks = append(blocks, hcl.ResourceBlock{
				Type:         "braintrust_project_tag",
				Name:         name,
				Comment:      t.Name,
				ID:           t.ID,
				Fields:       fields,
				LinkedFields: linked,
			})
		}
	}
	return blocks, nil
}

func buildProjectAutomationBlocks(ctx context.Context, projects []api.Project, resolver *hcl.ReferenceResolver) ([]hcl.ResourceBlock, error) {
	automations, err := apiClient.ListProjectAutomations(ctx)
	if err != nil {
		return nil, err
	}
	byProject := groupByProject(automations, func(automation api.ProjectAutomation) string { return automation.ProjectID })
	var blocks []hcl.ResourceBlock
	for _, p := range projects {
		for _, a := range byProject[p.ID] {
			name := hcl.ResourceNameFromIDAndLabel(a.ID, a.Name)
			fields := map[string]any{"name": a.Name}
			linked := make(map[string]string)

			if ref := resolver.Resolve("project_id", a.ProjectID); ref != "" {
				linked["project_id"] = ref
			} else {
				fields["project_id"] = a.ProjectID
			}
			if a.Description != nil {
				fields["description"] = *a.Description
			}

			// Unknown or malformed configurations must fail the export rather
			// than produce a successful-looking, incomplete resource.
			if len(a.Config) == 0 {
				return nil, fmt.Errorf("automation %s has no config", a.ID)
			}
			var config map[string]any
			if err := json.Unmarshal(a.Config, &config); err != nil {
				return nil, fmt.Errorf("decode automation %s config: %w", a.ID, err)
			}
			if err := writeAutomationConfig(fields, config); err != nil {
				return nil, fmt.Errorf("automation %s: %w", a.ID, err)
			}

			blocks = append(blocks, hcl.ResourceBlock{
				Type:         "braintrust_project_automation",
				Name:         name,
				Comment:      a.Name,
				ID:           a.ID,
				Fields:       fields,
				LinkedFields: linked,
			})
		}
	}
	return blocks, nil
}

func buildOrgAutomationBlocks(ctx context.Context, orgID string) ([]hcl.ResourceBlock, error) {
	automations, err := apiClient.ListOrgAutomations(ctx)
	if err != nil {
		return nil, err
	}
	var blocks []hcl.ResourceBlock
	for _, automation := range automations {
		if automation.OrgID != orgID {
			continue
		}
		var config struct {
			EventType     string `json:"event_type"`
			ObjectType    string `json:"object_type"`
			RetentionDays *int64 `json:"retention_days"`
		}
		if err := json.Unmarshal(automation.Config, &config); err != nil {
			return nil, fmt.Errorf("organization automation %s has invalid config: %w", automation.ID, err)
		}
		if config.EventType != "retention" || (config.ObjectType != "project_logs" && config.ObjectType != "experiment" && config.ObjectType != "dataset") || config.RetentionDays == nil || *config.RetentionDays < 0 {
			return nil, fmt.Errorf("organization automation %s has unsupported retention config", automation.ID)
		}
		var configFields map[string]json.RawMessage
		if err := json.Unmarshal(automation.Config, &configFields); err != nil || len(configFields) != 3 {
			return nil, fmt.Errorf("organization automation %s has unsupported config fields", automation.ID)
		}
		fields := map[string]any{
			"org_id": automation.OrgID,
			"name":   automation.Name,
			"retention_config": map[string]any{
				"object_type":    config.ObjectType,
				"retention_days": float64(*config.RetentionDays),
			},
		}
		if automation.Description != nil {
			fields["description"] = *automation.Description
		}
		blocks = append(blocks, hcl.ResourceBlock{
			Type: "braintrust_org_automation", Name: hcl.ResourceNameFromIDAndLabel(automation.ID, automation.Name),
			Comment: automation.Name, ID: automation.ID, Fields: fields,
		})
	}
	return blocks, nil
}

func buildMCPServerBlocks(ctx context.Context, projects []api.Project, resolver *hcl.ReferenceResolver) ([]hcl.ResourceBlock, error) {
	servers, err := apiClient.ListMCPServers(ctx)
	if err != nil {
		return nil, err
	}
	selectedProjects := make(map[string]bool, len(projects))
	for _, project := range projects {
		selectedProjects[project.ID] = true
	}
	var blocks []hcl.ResourceBlock
	for _, server := range servers {
		if !selectedProjects[server.ProjectID] || server.DeletedAt != nil {
			continue
		}
		fields := map[string]any{"name": server.Name, "url": server.URL}
		linked := map[string]string{}
		if ref := resolver.Resolve("project_id", server.ProjectID); ref != "" {
			linked["project_id"] = ref
		} else {
			fields["project_id"] = server.ProjectID
		}
		if server.Description != nil {
			fields["description"] = *server.Description
		}
		blocks = append(blocks, hcl.ResourceBlock{
			Type: "braintrust_mcp_server", Name: hcl.ResourceNameFromIDAndLabel(server.ID, server.Name),
			Comment: server.Name, ID: server.ID, Fields: fields, LinkedFields: linked,
		})
	}
	return blocks, nil
}

// writeAutomationConfig converts the raw automation config JSON into the
// Terraform schema structure. The API stores config as a single JSON object
// with an "event_type" discriminator. The Terraform provider splits this into
// one of the structured config fields, or config_json for newer API variants.
func writeAutomationConfig(fields map[string]any, config map[string]any) error {
	eventType, _ := config["event_type"].(string)

	switch eventType {
	case "logs":
		writeLogsConfig(fields, config)
	case "btql_export":
		writeExportConfig(fields, config)
	case "retention":
		writeRetentionConfig(fields, config)
	case "environment_update":
		writeEnvironmentUpdateConfig(fields, config)
	default:
		if eventType != "" {
			// Preserve new API variants in full until the provider has a typed
			// config for them. Canonical JSON matches the provider's imported
			// state and avoids a whitespace-only update after import.
			encoded, err := json.Marshal(config)
			if err != nil {
				return fmt.Errorf("encode automation config_json: %w", err)
			}
			fields["config_json"] = string(encoded)
		} else if _, ok := config["retention_days"]; ok {
			// Older API responses may have omitted event_type.
			writeRetentionConfig(fields, config)
		} else if _, ok := config["export_path"]; ok {
			writeExportConfig(fields, config)
		} else if _, ok := config["btql_filter"]; ok {
			writeLogsConfig(fields, config)
		} else {
			return fmt.Errorf("unsupported automation event_type %q", eventType)
		}
	}
	if logs, ok := fields["logs_config"].(map[string]any); ok {
		if logs["webhook_action"] == nil && logs["slack_action"] == nil {
			return fmt.Errorf("logs automation has no supported action")
		}
	}
	if update, ok := fields["environment_update_config"].(map[string]any); ok {
		if update["webhook_action"] == nil && update["slack_action"] == nil {
			return fmt.Errorf("environment-update automation has no supported action")
		}
	}
	if export, ok := fields["export_config"].(map[string]any); ok {
		credentials, _ := export["aws_iam_credentials"].(map[string]any)
		if credentials["role_arn"] == nil || credentials["external_id"] == nil {
			return fmt.Errorf("export automation lacks required AWS role ARN or external ID")
		}
	}
	if fields["logs_config"] == nil && fields["export_config"] == nil && fields["retention_config"] == nil && fields["environment_update_config"] == nil && fields["config_json"] == nil {
		return fmt.Errorf("automation config has no supported fields")
	}
	return nil
}

// writeLogsConfig handles event_type "logs" — alert on log events matching a filter.
func writeLogsConfig(fields map[string]any, config map[string]any) {
	logsConfig := make(map[string]any)
	if filter, ok := config["btql_filter"]; ok {
		logsConfig["btql_filter"] = filter
	}
	if interval, ok := config["interval_seconds"]; ok {
		logsConfig["interval_seconds"] = interval
	}

	// The API nests the action under an "action" key with a "type" discriminator.
	writeActionToConfig(logsConfig, config)

	if len(logsConfig) > 0 {
		fields["logs_config"] = logsConfig
	}
}

// writeExportConfig handles event_type "btql_export" — scheduled data export.
func writeExportConfig(fields map[string]any, config map[string]any) {
	exportConfig := make(map[string]any)

	if exportPath, ok := config["export_path"]; ok {
		exportConfig["export_path"] = exportPath
	}
	if format, ok := config["format"]; ok {
		exportConfig["format"] = format
	}
	if interval, ok := config["interval_seconds"]; ok {
		exportConfig["interval_seconds"] = interval
	}
	if batchSize, ok := config["batch_size"]; ok {
		exportConfig["batch_size"] = batchSize
	}

	// Parse export_definition to extract export_type and optional btql_query.
	if exportDef, ok := config["export_definition"].(map[string]any); ok {
		if exportType, ok := exportDef["type"].(string); ok {
			exportConfig["export_type"] = exportType
		}
		if btqlQuery, ok := exportDef["btql_query"].(string); ok && btqlQuery != "" {
			exportConfig["btql_query"] = btqlQuery
		}
	}

	// Parse credentials for cross-account S3 access.
	if creds, ok := config["credentials"].(map[string]any); ok {
		awsCreds := make(map[string]any)
		if roleARN, ok := creds["role_arn"].(string); ok {
			awsCreds["role_arn"] = roleARN
		}
		if externalID, ok := creds["external_id"].(string); ok && externalID != "" {
			awsCreds["external_id"] = externalID
		}
		if len(awsCreds) > 0 {
			exportConfig["aws_iam_credentials"] = awsCreds
		}
	}

	if len(exportConfig) > 0 {
		fields["export_config"] = exportConfig
	}
}

// writeRetentionConfig handles event_type "retention" — data retention policy.
func writeRetentionConfig(fields map[string]any, config map[string]any) {
	retConfig := make(map[string]any)
	if objType, ok := config["object_type"]; ok {
		retConfig["object_type"] = objType
	}
	if retentionDays, ok := config["retention_days"]; ok {
		retConfig["retention_days"] = retentionDays
	}
	if len(retConfig) > 0 {
		fields["retention_config"] = retConfig
	}
}

// writeEnvironmentUpdateConfig handles event_type "environment_update".
func writeEnvironmentUpdateConfig(fields map[string]any, config map[string]any) {
	envConfig := make(map[string]any)

	// Parse environment_filter list.
	if filters, ok := config["environment_filter"].([]any); ok && len(filters) > 0 {
		envConfig["environment_filter"] = filters
	}

	// Parse action (webhook or slack).
	writeActionToConfig(envConfig, config)

	if len(envConfig) > 0 {
		fields["environment_update_config"] = envConfig
	}
}

// writeActionToConfig parses the nested "action" object from the API config
// and writes it as either "webhook_action" or "slack_action" into the target map.
// The API stores: {"action": {"type": "webhook", "url": "..."}} or
// {"action": {"type": "slack", "workspace_id": "...", "channel": "...", ...}}.
func writeActionToConfig(target map[string]any, config map[string]any) {
	action, ok := config["action"].(map[string]any)
	if !ok {
		return
	}

	actionType, _ := action["type"].(string)
	switch actionType {
	case "webhook":
		webhookAction := make(map[string]any)
		if url, ok := action["url"].(string); ok {
			webhookAction["url"] = url
		}
		if len(webhookAction) > 0 {
			target["webhook_action"] = webhookAction
		}
	case "slack":
		slackAction := make(map[string]any)
		if wsID, ok := action["workspace_id"].(string); ok {
			slackAction["workspace_id"] = wsID
		}
		if ch, ok := action["channel"].(string); ok {
			slackAction["channel"] = ch
		}
		if tmpl, ok := action["message_template"].(string); ok && tmpl != "" {
			slackAction["message_template"] = tmpl
		}
		if len(slackAction) > 0 {
			target["slack_action"] = slackAction
		}
	}
}

func buildEnvVarBlocks(ctx context.Context, projects []api.Project, orgID string, resolver *hcl.ReferenceResolver) ([]hcl.ResourceBlock, []hcl.VariableBlock, error) {
	var blocks []hcl.ResourceBlock
	var vars []hcl.VariableBlock
	seen := make(map[string]bool)

	projectEnvVars, err := fetchProjectResources(ctx, projects, "env vars", func(ctx context.Context, projectID string) ([]api.EnvVar, error) {
		return apiClient.ListEnvVars(ctx, "project", projectID)
	})
	if err != nil {
		return nil, nil, err
	}
	for _, envVars := range projectEnvVars {
		for _, ev := range envVars {
			if seen[ev.ID] {
				continue
			}
			seen[ev.ID] = true

			name := hcl.ResourceNameFromIDAndLabel(ev.ID, ev.Name)
			fields := map[string]any{
				"name":        ev.Name,
				"object_type": ev.ObjectType,
				"object_id":   ev.ObjectID,
			}
			linked := make(map[string]string)
			if ref := resolver.Resolve("object_id", ev.ObjectID); ref != "" {
				linked["object_id"] = ref
				delete(fields, "object_id")
			}

			if ev.SecretType != nil {
				fields["secret_type"] = *ev.SecretType
			}
			// The API does not return values for either ordinary or typed env vars.
			varName := "braintrust_env_var_" + api.SanitizeName(ev.Name) + "_" + api.SanitizeName(ev.ID) + "_value"
			linked["value"] = "var." + varName
			vars = append(vars, hcl.VariableBlock{Name: varName, Description: fmt.Sprintf("Value for env var %q", ev.Name)})

			blocks = append(blocks, hcl.ResourceBlock{
				Type:         "braintrust_env_var",
				Name:         name,
				Comment:      ev.Name,
				ID:           ev.ID,
				Fields:       fields,
				LinkedFields: linked,
			})
		}
	}

	// Also fetch org-level env vars, including when the org has no projects.
	{
		orgVars, err := apiClient.ListEnvVars(ctx, "organization", orgID)
		if err != nil {
			return nil, nil, fmt.Errorf("list env vars for organization %s: %w", orgID, err)
		}
		for _, ev := range orgVars {
			if seen[ev.ID] {
				continue
			}
			seen[ev.ID] = true

			name := hcl.ResourceNameFromIDAndLabel(ev.ID, ev.Name)
			fields := map[string]any{
				"name":        ev.Name,
				"object_type": ev.ObjectType,
				"object_id":   ev.ObjectID,
			}
			linked := make(map[string]string)

			if ev.SecretType != nil {
				fields["secret_type"] = *ev.SecretType
			}
			varName := "braintrust_env_var_" + api.SanitizeName(ev.Name) + "_" + api.SanitizeName(ev.ID) + "_value"
			linked["value"] = "var." + varName
			vars = append(vars, hcl.VariableBlock{Name: varName, Description: fmt.Sprintf("Value for env var %q", ev.Name)})

			blocks = append(blocks, hcl.ResourceBlock{
				Type:         "braintrust_env_var",
				Name:         name,
				Comment:      ev.Name,
				ID:           ev.ID,
				Fields:       fields,
				LinkedFields: linked,
			})
		}
	}

	return blocks, vars, nil
}

func buildServiceTokenBlocks(ctx context.Context, orgID string, resolver *hcl.ReferenceResolver) ([]hcl.ResourceBlock, error) {
	tokens, err := apiClient.ListServiceTokens(ctx)
	if err != nil {
		return nil, err
	}
	var blocks []hcl.ResourceBlock
	for _, t := range tokens {
		if t.OrgID == nil || *t.OrgID != orgID {
			continue
		}
		name := hcl.ResourceNameFromIDAndLabel(t.ID, t.Name)
		fields := map[string]any{"name": t.Name}
		if t.ServiceAccountID != nil {
			fields["service_account_id"] = *t.ServiceAccountID
		}

		blocks = append(blocks, hcl.ResourceBlock{
			Type:    "braintrust_service_token",
			Name:    name,
			Comment: t.Name,
			ID:      t.ID,
			Fields:  fields,
		})
	}
	return blocks, nil
}

func buildAISecretBlocks(ctx context.Context, orgID string, resolver *hcl.ReferenceResolver) ([]hcl.ResourceBlock, []hcl.VariableBlock, error) {
	secrets, err := apiClient.ListAISecrets(ctx)
	if err != nil {
		return nil, nil, err
	}
	var blocks []hcl.ResourceBlock
	var vars []hcl.VariableBlock

	for _, s := range secrets {
		if s.OrgID != orgID {
			continue
		}
		name := hcl.ResourceNameFromIDAndLabel(s.ID, s.Name)
		fields := map[string]any{"name": s.Name}
		linked := make(map[string]string)

		if s.Type != nil {
			fields["type"] = *s.Type
		}

		// Secret value is write-only — use a variable reference.
		varName := "braintrust_ai_secret_" + api.SanitizeName(s.Name) + "_" + api.SanitizeName(s.ID) + "_secret"
		linked["secret"] = "var." + varName
		vars = append(vars, hcl.VariableBlock{
			Name:        varName,
			Description: fmt.Sprintf("API key/secret for AI provider %q", s.Name),
		})

		blocks = append(blocks, hcl.ResourceBlock{
			Type:         "braintrust_ai_secret",
			Name:         name,
			Comment:      s.Name,
			ID:           s.ID,
			Fields:       fields,
			LinkedFields: linked,
		})
	}
	return blocks, vars, nil
}

// =============================================================================
// Helpers
// =============================================================================

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func canonicalJSON(raw json.RawMessage) (string, error) {
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", err
	}
	canonical, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return string(canonical), nil
}

func resolveOrgID(ctx context.Context, projects []api.Project, explicit string) (string, error) {
	if explicit != "" {
		if _, err := apiClient.GetOrganization(ctx, explicit); err != nil {
			return "", fmt.Errorf("verify selected organization %s: %w", explicit, err)
		}
		return explicit, nil
	}
	orgIDs := make(map[string]bool)
	for _, project := range projects {
		if project.OrgID != "" {
			orgIDs[project.OrgID] = true
		}
	}
	if len(orgIDs) != 1 {
		return "", fmt.Errorf("cannot infer one organization from %d visible organization(s); set --org-id or BRAINTRUST_ORG_ID", len(orgIDs))
	}
	for orgID := range orgIDs {
		return orgID, nil
	}
	return "", fmt.Errorf("cannot infer an organization; set --org-id or BRAINTRUST_ORG_ID")
}

func filterProjects(projects []api.Project, filter []string) ([]api.Project, error) {
	if len(filter) == 0 {
		return projects, nil
	}
	filterSet := make(map[string]bool)
	for _, f := range filter {
		filterSet[strings.ToLower(f)] = true
	}

	var filtered []api.Project
	for _, p := range projects {
		if filterSet[strings.ToLower(p.ID)] || filterSet[strings.ToLower(p.Name)] {
			filtered = append(filtered, p)
			delete(filterSet, strings.ToLower(p.ID))
			delete(filterSet, strings.ToLower(p.Name))
		}
	}
	if len(filterSet) > 0 {
		var missing []string
		for name := range filterSet {
			missing = append(missing, name)
		}
		slices.Sort(missing)
		return nil, fmt.Errorf("project selector(s) not found in selected organization: %s", strings.Join(missing, ", "))
	}
	return filtered, nil
}
