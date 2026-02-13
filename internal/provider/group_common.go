package provider

import (
	"github.com/braintrustdata/terraform-provider-braintrust/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// groupModel is the shared data model for both the group resource and data source.
type groupModel struct {
	ID           types.String `tfsdk:"id"`
	OrgID        types.String `tfsdk:"org_id"`
	Name         types.String `tfsdk:"name"`
	Description  types.String `tfsdk:"description"`
	Created      types.String `tfsdk:"created"`
	DeletedAt    types.String `tfsdk:"deleted_at"`
	UserID       types.String `tfsdk:"user_id"`
	MemberUsers  types.Set    `tfsdk:"member_users"`
	MemberGroups types.Set    `tfsdk:"member_groups"`
}

// mapGroupToModel maps a client.Group to the shared Terraform model.
func mapGroupToModel(group *client.Group, model *groupModel) {
	model.ID = types.StringValue(group.ID)
	model.OrgID = types.StringValue(group.OrgID)
	model.Name = types.StringValue(group.Name)
	model.Description = stringPtrToTFValue(group.Description)
	model.Created = stringPtrToTFValue(group.Created)
	model.DeletedAt = stringPtrToTFValue(group.DeletedAt)
	model.UserID = stringPtrToTFValue(group.UserID)

	model.MemberUsers = stringSliceToTFSet(group.MemberUsers)
	model.MemberGroups = stringSliceToTFSet(group.MemberGroups)
}
