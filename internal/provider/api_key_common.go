package provider

import (
	"github.com/braintrustdata/terraform-provider-braintrust/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// apiKeyModel is the Terraform data model for the API key resource.
type apiKeyModel struct {
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	PreviewName types.String `tfsdk:"preview_name"`
	Key         types.String `tfsdk:"key"`
	Created     types.String `tfsdk:"created"`
	UserID      types.String `tfsdk:"user_id"`
	OrgID       types.String `tfsdk:"org_id"`
}

// mapAPIKeyToModel maps a client.APIKey (GET response) to the Terraform model.
// Note: Key is NOT set here because the GET response doesn't include it.
// The raw key is only available at creation time.
func mapAPIKeyToModel(apiKey *client.APIKey, model *apiKeyModel) {
	model.ID = types.StringValue(apiKey.ID)
	model.Name = types.StringValue(apiKey.Name)
	model.PreviewName = types.StringValue(apiKey.PreviewName)
	model.Created = stringPtrToTFValue(apiKey.Created)
	model.UserID = stringPtrToTFValue(apiKey.UserID)
	model.OrgID = stringPtrToTFValue(apiKey.OrgID)
	// model.Key is intentionally NOT set -- see doc comment.
}

// mapCreateAPIKeyOutputToModel maps a client.CreateAPIKeyOutput (POST response) to the model.
// This is the only time we get the raw key.
func mapCreateAPIKeyOutputToModel(output *client.CreateAPIKeyOutput, model *apiKeyModel) {
	model.ID = types.StringValue(output.ID)
	model.Name = types.StringValue(output.Name)
	model.PreviewName = types.StringValue(output.PreviewName)
	model.Key = types.StringValue(output.Key)
	model.Created = stringPtrToTFValue(output.Created)
	model.UserID = stringPtrToTFValue(output.UserID)
	model.OrgID = stringPtrToTFValue(output.OrgID)
}
