package provider

import (
	"github.com/braintrustdata/terraform-provider-braintrust/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type projectGroupModel struct {
	ID             types.String `tfsdk:"id"`
	OrgID          types.String `tfsdk:"org_id"`
	Name           types.String `tfsdk:"name"`
	Description    types.String `tfsdk:"description"`
	Created        types.String `tfsdk:"created"`
	DeletedAt      types.String `tfsdk:"deleted_at"`
	UserID         types.String `tfsdk:"user_id"`
	MemberProjects types.Set    `tfsdk:"member_projects"`
}

func mapProjectGroupToModel(group *client.ProjectGroup, model *projectGroupModel) {
	model.ID = types.StringValue(group.ID)
	model.OrgID = types.StringValue(group.OrgID)
	model.Name = types.StringValue(group.Name)
	model.Description = stringPtrToTFValue(group.Description)
	model.Created = stringPtrToTFValue(group.Created)
	model.DeletedAt = stringPtrToTFValue(group.DeletedAt)
	model.UserID = stringPtrToTFValue(group.UserID)
	model.MemberProjects = stringSliceToTFSet(group.MemberProjects)
}
