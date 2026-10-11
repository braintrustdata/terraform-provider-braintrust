package provider

import (
	"github.com/braintrustdata/terraform-provider-braintrust/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// projectScoreModel is the shared data model for the project score resource.
type projectScoreModel struct {
	ID             types.String `tfsdk:"id"`
	ProjectID      types.String `tfsdk:"project_id"`
	UserID         types.String `tfsdk:"user_id"`
	Name           types.String `tfsdk:"name"`
	Description    types.String `tfsdk:"description"`
	ScoreType      types.String `tfsdk:"score_type"`
	CategoriesJSON types.String `tfsdk:"categories_json"`
	ConfigJSON     types.String `tfsdk:"config_json"`
	Created        types.String `tfsdk:"created"`
}

// mapProjectScoreToModel maps a client.ProjectScore to the Terraform model.
func mapProjectScoreToModel(score *client.ProjectScore, model *projectScoreModel) {
	model.ID = types.StringValue(score.ID)
	model.ProjectID = types.StringValue(score.ProjectID)
	model.UserID = stringPtrToTFValue(score.UserID)
	model.Name = types.StringValue(score.Name)
	model.Description = stringPtrToTFValue(score.Description)
	model.ScoreType = types.StringValue(score.ScoreType)
	model.CategoriesJSON = jsonState(score.Categories, model.CategoriesJSON)
	model.ConfigJSON = jsonState(score.Config, model.ConfigJSON)
	model.Created = stringPtrToTFValue(score.Created)
}
