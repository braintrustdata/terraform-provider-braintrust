package provider

import (
	"github.com/braintrustdata/terraform-provider-braintrust/internal/client"
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

// mapProjectTagToModel maps a client.ProjectTag to the Terraform model.
func mapProjectTagToModel(tag *client.ProjectTag, model *projectTagModel) {
	model.ID = types.StringValue(tag.ID)
	model.ProjectID = types.StringValue(tag.ProjectID)
	model.UserID = types.StringValue(tag.UserID)
	model.Name = types.StringValue(tag.Name)
	model.Description = stringPtrToTFValue(tag.Description)
	model.Color = stringPtrToTFValue(tag.Color)
	model.Created = stringPtrToTFValue(tag.Created)
}
