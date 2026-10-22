package provider

import (
	"fmt"

	"github.com/braintrustdata/terraform-provider-braintrust/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// aclObjectTypes are the valid values for ACL object_type and restrict_object_type fields.
var aclObjectTypes = []string{
	"organization", "project", "experiment", "dataset", "prompt",
	"prompt_session", "group", "role", "org_member", "project_log", "org_project",
	"org_audit_logs", "project_group", "project_group_projects", "ai_secret",
	"org_ai_secret", "org_account",
}

// permissionTypes are the valid permission values for ACLs and roles.
var permissionTypes = []string{
	"create", "read", "update", "delete",
	"create_acls", "read_acls", "update_acls", "delete_acls",
}

// clientFromProviderData extracts the *client.Client from provider data.
// Returns nil with no diagnostics if providerData is nil (normal during early
// provider lifecycle calls). Returns nil with an error diagnostic if the type
// assertion fails.
func clientFromProviderData(providerData any, diags *diag.Diagnostics) *client.Client {
	if providerData == nil {
		return nil
	}

	c, ok := providerData.(*client.Client)
	if !ok {
		diags.AddError(
			"Unexpected Provider Data Type",
			fmt.Sprintf("Expected *client.Client, got: %T. Please report this issue to the provider developers.", providerData),
		)
		return nil
	}
	return c
}

// requireClient checks that the client is non-nil and adds a diagnostic if not.
// Returns true if the client is ready to use.
func requireClient(c *client.Client, diags *diag.Diagnostics) bool {
	if c == nil {
		diags.AddError(
			"Provider Not Configured",
			"The provider client is not configured. This is a provider bug.",
		)
		return false
	}
	return true
}

// stringPtrToTFValue converts a *string to a types.String Terraform value.
// Returns types.StringNull() if the pointer is nil.
func stringPtrToTFValue(s *string) types.String {
	if s != nil {
		return types.StringValue(*s)
	}
	return types.StringNull()
}

// tfStringToPtr converts a types.String to a *string for API requests.
// Returns nil if the value is null or unknown.
func tfStringToPtr(s types.String) *string {
	if s.IsNull() || s.IsUnknown() {
		return nil
	}
	v := s.ValueString()
	return &v
}

// computedStringAttribute returns a Computed string schema attribute with
// UseStateForUnknown, suitable for server-assigned fields like id, org_id, etc.
func computedStringAttribute(description string) schema.StringAttribute {
	return schema.StringAttribute{
		Computed:    true,
		Description: description,
		PlanModifiers: []planmodifier.String{
			stringplanmodifier.UseStateForUnknown(),
		},
	}
}

// optionalComputedStringAttribute returns an Optional+Computed string schema
// attribute with UseStateForUnknown, suitable for fields that the user can set
// but the server may also provide a default for.
func optionalComputedStringAttribute(description string) schema.StringAttribute {
	return schema.StringAttribute{
		Optional:    true,
		Computed:    true,
		Description: description,
		PlanModifiers: []planmodifier.String{
			stringplanmodifier.UseStateForUnknown(),
		},
	}
}

// diffStringSlices computes the elements to add and remove to go from current to desired.
func diffStringSlices(current, desired []string) (add, remove []string) {
	currentSet := make(map[string]bool, len(current))
	for _, s := range current {
		currentSet[s] = true
	}
	desiredSet := make(map[string]bool, len(desired))
	for _, s := range desired {
		desiredSet[s] = true
	}

	for _, s := range desired {
		if !currentSet[s] {
			add = append(add, s)
		}
	}
	for _, s := range current {
		if !desiredSet[s] {
			remove = append(remove, s)
		}
	}
	return add, remove
}

// memberPermissionKey returns a unique string key for a member permission.
func memberPermissionKey(mp memberPermission) string {
	key := string(mp.Permission)
	if mp.RestrictObjectType != nil {
		key += ":" + string(*mp.RestrictObjectType)
	}
	return key
}

// stringSliceToTFList converts a []string to a types.List of StringType.
// Returns an empty list (not null) when the input is nil or empty.
func stringSliceToTFList(ss []string) types.List {
	if len(ss) == 0 {
		return types.ListValueMust(types.StringType, []attr.Value{})
	}
	elems := make([]attr.Value, len(ss))
	for i, s := range ss {
		elems[i] = types.StringValue(s)
	}
	return types.ListValueMust(types.StringType, elems)
}

// stringListFromModel extracts a []string from a types.List of strings.
// Returns nil if the list is null, unknown, or empty.
func stringListFromModel(list types.List) []string {
	if list.IsNull() || list.IsUnknown() || len(list.Elements()) == 0 {
		return nil
	}
	result := make([]string, len(list.Elements()))
	for i, elem := range list.Elements() {
		s, ok := elem.(types.String)
		if !ok {
			continue
		}
		result[i] = s.ValueString()
	}
	return result
}

// stringSliceToTFSet converts a Go string slice to a Terraform types.Set of strings.
func stringSliceToTFSet(ss []string) types.Set {
	if len(ss) == 0 {
		return types.SetValueMust(types.StringType, []attr.Value{})
	}
	elems := make([]attr.Value, len(ss))
	for i, s := range ss {
		elems[i] = types.StringValue(s)
	}
	return types.SetValueMust(types.StringType, elems)
}

// stringSetFromModel extracts a []string from a types.Set of strings.
// Returns nil if the set is null, unknown, or empty.
func stringSetFromModel(set types.Set) []string {
	if set.IsNull() || set.IsUnknown() || len(set.Elements()) == 0 {
		return nil
	}
	result := make([]string, len(set.Elements()))
	for i, elem := range set.Elements() {
		s, ok := elem.(types.String)
		if !ok {
			continue
		}
		result[i] = s.ValueString()
	}
	return result
}

// diffMemberPermissions computes the permissions to add and remove.
func diffMemberPermissions(current, desired []memberPermission) (add, remove []memberPermission) {
	currentSet := make(map[string]bool, len(current))
	for _, mp := range current {
		currentSet[memberPermissionKey(mp)] = true
	}
	desiredSet := make(map[string]bool, len(desired))
	for _, mp := range desired {
		desiredSet[memberPermissionKey(mp)] = true
	}

	for _, mp := range desired {
		if !currentSet[memberPermissionKey(mp)] {
			add = append(add, mp)
		}
	}
	for _, mp := range current {
		if !desiredSet[memberPermissionKey(mp)] {
			remove = append(remove, mp)
		}
	}
	return add, remove
}
