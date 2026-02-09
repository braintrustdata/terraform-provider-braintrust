package provider

import (
	"sort"

	"github.com/braintrustdata/terraform-provider-braintrust/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// roleModel is the shared data model for both the role resource and data source.
type roleModel struct {
	ID                types.String            `tfsdk:"id"`
	OrgID             types.String            `tfsdk:"org_id"`
	Name              types.String            `tfsdk:"name"`
	Description       types.String            `tfsdk:"description"`
	Created           types.String            `tfsdk:"created"`
	DeletedAt         types.String            `tfsdk:"deleted_at"`
	UserID            types.String            `tfsdk:"user_id"`
	MemberPermissions []memberPermissionModel `tfsdk:"member_permissions"`
	MemberRoles       types.List              `tfsdk:"member_roles"`
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

	// Map member_permissions, sorted deterministically by (permission, restrict_object_type)
	// so that the API's return order doesn't cause spurious diffs.
	if len(role.MemberPermissions) > 0 {
		sorted := make([]client.MemberPermission, len(role.MemberPermissions))
		copy(sorted, role.MemberPermissions)
		sort.Slice(sorted, func(i, j int) bool {
			if sorted[i].Permission != sorted[j].Permission {
				return sorted[i].Permission < sorted[j].Permission
			}
			ri, rj := "", ""
			if sorted[i].RestrictObjectType != nil {
				ri = *sorted[i].RestrictObjectType
			}
			if sorted[j].RestrictObjectType != nil {
				rj = *sorted[j].RestrictObjectType
			}
			return ri < rj
		})

		perms := make([]memberPermissionModel, len(sorted))
		for i, mp := range sorted {
			perms[i] = memberPermissionModel{
				Permission:         types.StringValue(mp.Permission),
				RestrictObjectType: stringPtrToTFValue(mp.RestrictObjectType),
			}
		}
		model.MemberPermissions = perms
	} else {
		model.MemberPermissions = []memberPermissionModel{}
	}

	// Map member_roles.
	model.MemberRoles = stringSliceToTFList(role.MemberRoles)
}

// memberPermissionsFromModel converts the TF model to client structs.
func memberPermissionsFromModel(perms []memberPermissionModel) []client.MemberPermission {
	if len(perms) == 0 {
		return nil
	}
	result := make([]client.MemberPermission, len(perms))
	for i, p := range perms {
		result[i] = client.MemberPermission{
			Permission:         p.Permission.ValueString(),
			RestrictObjectType: tfStringToPtr(p.RestrictObjectType),
		}
	}
	return result
}

