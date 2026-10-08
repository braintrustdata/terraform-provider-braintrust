package provider

import (
	"context"

	"github.com/braintrustdata/terraform-provider-braintrust/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var rolePermissionObjectType = types.ObjectType{AttrTypes: map[string]attr.Type{
	"permission":           types.StringType,
	"restrict_object_type": types.StringType,
}}

// roleModel is the shared data model for both the role resource and data source.
type roleModel struct {
	ID                types.String `tfsdk:"id"`
	OrgID             types.String `tfsdk:"org_id"`
	Name              types.String `tfsdk:"name"`
	Description       types.String `tfsdk:"description"`
	Created           types.String `tfsdk:"created"`
	DeletedAt         types.String `tfsdk:"deleted_at"`
	UserID            types.String `tfsdk:"user_id"`
	MemberPermissions types.Set    `tfsdk:"member_permissions"`
	MemberRoles       types.Set    `tfsdk:"member_roles"`
}

// memberPermissionModel represents a single (permission, restrict_object_type) tuple.
type memberPermissionModel struct {
	Permission         types.String `tfsdk:"permission"`
	RestrictObjectType types.String `tfsdk:"restrict_object_type"`
}

// mapRoleToModel maps a client.Role to the shared Terraform model.
func mapRoleToModel(role *client.Role, model *roleModel) {
	model.ID = types.StringValue(role.ID)
	model.OrgID = stringPtrToTFValue(role.OrgID)
	model.Name = types.StringValue(role.Name)
	model.Description = stringPtrToTFValue(role.Description)
	model.Created = stringPtrToTFValue(role.Created)
	model.DeletedAt = stringPtrToTFValue(role.DeletedAt)
	model.UserID = stringPtrToTFValue(role.UserID)

	// Map member_permissions. Since this is a Set in the schema, Terraform
	// handles ordering; no need to sort.
	perms := make([]attr.Value, 0, len(role.MemberPermissions))
	for _, mp := range role.MemberPermissions {
		perms = append(perms, types.ObjectValueMust(rolePermissionObjectType.AttrTypes, map[string]attr.Value{
			"permission":           types.StringValue(mp.Permission),
			"restrict_object_type": stringPtrToTFValue(mp.RestrictObjectType),
		}))
	}
	model.MemberPermissions = types.SetValueMust(rolePermissionObjectType, perms)

	// Map member_roles.
	model.MemberRoles = stringSliceToTFSet(role.MemberRoles)
}

// memberPermissionsFromModel converts the TF model to client structs.
func memberPermissionsFromModel(ctx context.Context, value types.Set) ([]client.MemberPermission, diag.Diagnostics) {
	if value.IsNull() || value.IsUnknown() {
		return nil, nil
	}
	var perms []memberPermissionModel
	diags := value.ElementsAs(ctx, &perms, false)
	if diags.HasError() {
		return nil, diags
	}
	result := make([]client.MemberPermission, len(perms))
	for i, p := range perms {
		result[i] = client.MemberPermission{
			Permission:         p.Permission.ValueString(),
			RestrictObjectType: tfStringToPtr(p.RestrictObjectType),
		}
	}
	return result, diags
}
