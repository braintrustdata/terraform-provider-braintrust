package provider

import (
	"github.com/braintrustdata/terraform-provider-braintrust/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// serviceTokenModel is the Terraform data model for the service token resource.
type serviceTokenModel struct {
	ID                  types.String `tfsdk:"id"`
	Name                types.String `tfsdk:"name"`
	PreviewName         types.String `tfsdk:"preview_name"`
	Key                 types.String `tfsdk:"key"`
	Created             types.String `tfsdk:"created"`
	ServiceAccountID    types.String `tfsdk:"service_account_id"`
	ServiceAccountEmail types.String `tfsdk:"service_account_email"`
	ServiceAccountName  types.String `tfsdk:"service_account_name"`
	OrgID               types.String `tfsdk:"org_id"`
	ExpiresInSeconds    types.Int64  `tfsdk:"expires_in_seconds"`
	ExpiresAt           types.String `tfsdk:"expires_at"`
}

// mapServiceTokenToModel maps a client.ServiceToken (GET response) to the Terraform model.
// Note: Key is NOT set here because the GET response doesn't include it.
// The raw key is only available at creation time.
func mapServiceTokenToModel(token *client.ServiceToken, model *serviceTokenModel) {
	model.ID = types.StringValue(token.ID)
	model.Name = types.StringValue(token.Name)
	model.PreviewName = types.StringValue(token.PreviewName)
	model.Created = stringPtrToTFValue(token.Created)
	model.ServiceAccountID = stringPtrToTFValue(token.ServiceAccountID)
	model.ServiceAccountEmail = stringPtrToTFValue(token.ServiceAccountEmail)
	model.ServiceAccountName = stringPtrToTFValue(token.ServiceAccountName)
	model.OrgID = stringPtrToTFValue(token.OrgID)
	model.ExpiresAt = stringPtrToTFValue(token.ExpiresAt)
	// model.Key is intentionally NOT set -- see doc comment.
}

// mapCreateServiceTokenOutputToModel maps a client.CreateServiceTokenOutput (POST response) to the model.
// This is the only time we get the raw key.
func mapCreateServiceTokenOutputToModel(output *client.CreateServiceTokenOutput, model *serviceTokenModel) {
	model.ID = types.StringValue(output.ID)
	model.Name = types.StringValue(output.Name)
	model.PreviewName = types.StringValue(output.PreviewName)
	model.Key = types.StringValue(output.Key)
	model.Created = stringPtrToTFValue(output.Created)
	model.ServiceAccountID = stringPtrToTFValue(output.ServiceAccountID)
	model.ServiceAccountEmail = stringPtrToTFValue(output.ServiceAccountEmail)
	model.ServiceAccountName = stringPtrToTFValue(output.ServiceAccountName)
	model.OrgID = stringPtrToTFValue(output.OrgID)
	model.ExpiresAt = stringPtrToTFValue(output.ExpiresAt)
}
