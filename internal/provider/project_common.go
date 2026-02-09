package provider

import (
	"github.com/braintrustdata/terraform-provider-braintrust/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// projectModel is the shared data model for both the project resource and data source.
type projectModel struct {
	ID                   types.String `tfsdk:"id"`
	OrgID                types.String `tfsdk:"org_id"`
	Name                 types.String `tfsdk:"name"`
	Description          types.String `tfsdk:"description"`
	Created              types.String `tfsdk:"created"`
	DeletedAt            types.String `tfsdk:"deleted_at"`
	UserID               types.String `tfsdk:"user_id"`
	ComparisonKey        types.String `tfsdk:"comparison_key"`
	BaselineExperimentID types.String `tfsdk:"baseline_experiment_id"`
}

// mapProjectToModel maps a client.Project to the shared Terraform model.
// Both project and model must be non-nil.
func mapProjectToModel(project *client.Project, model *projectModel) {
	model.ID = types.StringValue(project.ID)
	model.OrgID = types.StringValue(project.OrgID)
	model.Name = types.StringValue(project.Name)

	model.Description = stringPtrToTFValue(project.Description)
	model.Created = stringPtrToTFValue(project.Created)
	model.DeletedAt = stringPtrToTFValue(project.DeletedAt)
	model.UserID = stringPtrToTFValue(project.UserID)

	// Map settings fields.
	if project.Settings != nil {
		model.ComparisonKey = stringPtrToTFValue(project.Settings.ComparisonKey)
		model.BaselineExperimentID = stringPtrToTFValue(project.Settings.BaselineExperimentID)
	} else {
		model.ComparisonKey = types.StringNull()
		model.BaselineExperimentID = types.StringNull()
	}
}
