package provider

import (
	"context"

	"github.com/braintrustdata/braintrust-sdk-go/openapi"
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

// memberPermission matches the SDK's inline member permission struct, so
// slices of it can be assigned to SDK role fields directly.
type memberPermission = struct {
	Permission         openapi.Permission     `json:"permission"`
	RestrictObjectType *openapi.AclObjectType `json:"restrict_object_type,omitempty"`
}

// mapRoleToModel maps an SDK role to the shared Terraform model.
func mapRoleToModel(role *openapi.Role, model *roleModel) {
	model.ID = types.StringValue(role.Id.String())
	model.OrgID = nullableUUIDToTF(role.OrgId)
	model.Name = types.StringValue(role.Name)
	model.Description = nullableStringToTF(role.Description)
	model.Created = nullableTimeToTF(role.Created, model.Created)
	model.DeletedAt = nullableTimeToTF(role.DeletedAt, model.DeletedAt)
	model.UserID = nullableUUIDToTF(role.UserId)

	// Map member_permissions. Since this is a Set in the schema, Terraform
	// handles ordering; no need to sort.
	memberPermissions := nullableSliceValue(role.MemberPermissions)
	perms := make([]attr.Value, 0, len(memberPermissions))
	for _, mp := range memberPermissions {
		restrict := types.StringNull()
		if mp.RestrictObjectType != nil {
			restrict = types.StringValue(string(*mp.RestrictObjectType))
		}
		perms = append(perms, types.ObjectValueMust(rolePermissionObjectType.AttrTypes, map[string]attr.Value{
			"permission":           types.StringValue(string(mp.Permission)),
			"restrict_object_type": restrict,
		}))
	}
	model.MemberPermissions = types.SetValueMust(rolePermissionObjectType, perms)

	// Map member_roles.
	model.MemberRoles = stringSliceToTFSet(uuidStrings(nullableSliceValue(role.MemberRoles)))
}

func getRole(ctx context.Context, c *client.Client, id string) (*openapi.Role, error) {
	resp, err := c.API.GetRoleIdWithResponse(ctx, id)
	if err != nil {
		return nil, err
	}
	return client.JSON(resp.JSON200)
}

// memberPermissionsFromModel converts the TF model to SDK structs.
func memberPermissionsFromModel(ctx context.Context, value types.Set) ([]memberPermission, diag.Diagnostics) {
	if value.IsNull() || value.IsUnknown() {
		return nil, nil
	}
	var perms []memberPermissionModel
	diags := value.ElementsAs(ctx, &perms, false)
	if diags.HasError() {
		return nil, diags
	}
	result := make([]memberPermission, len(perms))
	for i, p := range perms {
		result[i] = memberPermission{Permission: openapi.Permission(p.Permission.ValueString())}
		if restrict := tfStringToPtr(p.RestrictObjectType); restrict != nil {
			objectType := openapi.AclObjectType(*restrict)
			result[i].RestrictObjectType = &objectType
		}
	}
	return result, diags
}
