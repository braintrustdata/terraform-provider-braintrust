package provider

import (
	"context"

	"github.com/braintrustdata/braintrust-sdk-go/openapi"
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

// mapServiceTokenToModel maps an SDK service token (GET response) to the Terraform model.
// Note: Key is NOT set here because the GET response doesn't include it.
// The raw key is only available at creation time.
func mapServiceTokenToModel(token *openapi.ServiceToken, model *serviceTokenModel) {
	model.ID = types.StringValue(token.Id.String())
	model.Name = types.StringValue(token.Name)
	model.PreviewName = types.StringValue(token.PreviewName)
	model.Created = nullableTimeToTF(token.Created, model.Created)
	model.ServiceAccountID = nullableUUIDToTF(token.ServiceAccountId)
	model.ServiceAccountEmail = nullableStringToTF(token.ServiceAccountEmail)
	model.ServiceAccountName = nullableStringToTF(token.ServiceAccountName)
	model.OrgID = nullableUUIDToTF(token.OrgId)
	model.ExpiresAt = nullableTimeToTF(token.ExpiresAt, model.ExpiresAt)
	// model.Key is intentionally NOT set -- see doc comment.
}

// mapCreateServiceTokenOutputToModel maps an SDK create output (POST response) to the model.
// This is the only time we get the raw key.
func mapCreateServiceTokenOutputToModel(output *openapi.CreateServiceTokenOutput, model *serviceTokenModel) {
	model.ID = types.StringValue(output.Id.String())
	model.Name = types.StringValue(output.Name)
	model.PreviewName = types.StringValue(output.PreviewName)
	model.Key = types.StringValue(output.Key)
	model.Created = nullableTimeToTF(output.Created, model.Created)
	model.ServiceAccountID = nullableUUIDToTF(output.ServiceAccountId)
	model.ServiceAccountEmail = nullableStringToTF(output.ServiceAccountEmail)
	model.ServiceAccountName = nullableStringToTF(output.ServiceAccountName)
	model.OrgID = nullableUUIDToTF(output.OrgId)
	model.ExpiresAt = nullableTimeToTF(output.ExpiresAt, model.ExpiresAt)
}

func getServiceToken(ctx context.Context, c *client.Client, id string) (*openapi.ServiceToken, error) {
	resp, err := c.API.GetServiceTokenIdWithResponse(ctx, id)
	if err != nil {
		return nil, err
	}
	return client.JSON(resp.JSON200)
}

// listServiceTokensByName lists service tokens by name. The spec does not
// declare the endpoint's name filter, so it is added to each request.
func listServiceTokensByName(ctx context.Context, c *client.Client, name string) ([]openapi.ServiceToken, error) {
	return client.ListObjects("/v1/service_token", func(limit int, startingAfter *string) (*client.Page[openapi.ServiceToken], error) {
		page, err := c.API.GetServiceTokenWithResponse(ctx, &openapi.GetServiceTokenParams{Limit: &limit, StartingAfter: startingAfter}, client.WithQuery("name", name))
		if err != nil {
			return nil, err
		}
		return page.JSON200, nil
	}, func(token openapi.ServiceToken) string { return token.Id.String() })
}

// serviceTokenMatches reports whether token has the name and service account.
func serviceTokenMatches(token openapi.ServiceToken, name, serviceAccountID string) bool {
	return token.Name == name && nullableUUIDToTF(token.ServiceAccountId).ValueString() == serviceAccountID
}
