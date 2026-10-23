package provider

import (
	"context"

	"github.com/braintrustdata/braintrust-sdk-go/openapi"
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

// projectScore keeps categories and config as raw JSON, which Terraform
// passes through verbatim. The SDK's config model would drop unknown keys.
type projectScore = client.Raw[openapi.ProjectScore]

func getProjectScore(ctx context.Context, c *client.Client, id string) (*projectScore, error) {
	resp, err := c.API.GetProjectScoreIdWithResponse(ctx, id)
	if err != nil {
		return nil, err
	}
	return client.DecodeRaw(resp.JSON200, resp.Body)
}

// mapProjectScoreToModel maps a project score to the Terraform model.
func mapProjectScoreToModel(raw *projectScore, model *projectScoreModel) {
	score := raw.Value
	model.ID = types.StringValue(score.Id.String())
	model.ProjectID = types.StringValue(score.ProjectId.String())
	model.UserID = optionalUUIDToTF(score.UserId)
	model.Name = types.StringValue(score.Name)
	model.Description = nullableStringToTF(score.Description)
	model.ScoreType = types.StringValue(string(score.ScoreType))
	model.CategoriesJSON = jsonState(raw.Fields["categories"], model.CategoriesJSON)
	model.ConfigJSON = jsonState(raw.Fields["config"], model.ConfigJSON)
	model.Created = nullableTimeToTF(score.Created, model.Created)
}
