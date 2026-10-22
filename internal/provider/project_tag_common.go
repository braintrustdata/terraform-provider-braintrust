package provider

import (
	"github.com/braintrustdata/braintrust-sdk-go/openapi"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// projectTagModel is the shared data model for the project tag resource.
type projectTagModel struct {
	ID          types.String `tfsdk:"id"`
	ProjectID   types.String `tfsdk:"project_id"`
	UserID      types.String `tfsdk:"user_id"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
	Color       types.String `tfsdk:"color"`
	Created     types.String `tfsdk:"created"`
}

// mapProjectTagToModel maps an SDK project tag to the Terraform model.
func mapProjectTagToModel(tag *openapi.ProjectTag, model *projectTagModel) {
	model.ID = types.StringValue(tag.Id.String())
	model.ProjectID = types.StringValue(tag.ProjectId.String())
	model.UserID = types.StringValue(tag.UserId.String())
	model.Name = types.StringValue(tag.Name)
	model.Description = nullableStringToTF(tag.Description)
	model.Color = nullableStringToTF(tag.Color)
	model.Created = nullableTimeToTF(tag.Created, model.Created)
}
