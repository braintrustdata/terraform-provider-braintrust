package provider

import (
	"context"
	"errors"
	"fmt"

	"github.com/braintrustdata/braintrust-sdk-go/openapi"
	"github.com/braintrustdata/terraform-provider-braintrust/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type apiKeyModel struct {
	ID               types.String `tfsdk:"id"`
	Name             types.String `tfsdk:"name"`
	OrgID            types.String `tfsdk:"org_id"`
	Key              types.String `tfsdk:"key"`
	ExpiresInSeconds types.Int64  `tfsdk:"expires_in_seconds"`
	Created          types.String `tfsdk:"created"`
	PreviewName      types.String `tfsdk:"preview_name"`
	UserID           types.String `tfsdk:"user_id"`
	UserEmail        types.String `tfsdk:"user_email"`
	UserGivenName    types.String `tfsdk:"user_given_name"`
	UserFamilyName   types.String `tfsdk:"user_family_name"`
	ExpiresAt        types.String `tfsdk:"expires_at"`
}

// GET never returns the raw key or requested lifetime. Preserve both from state;
// the actual, possibly capped expiration is reported separately in expires_at.
func mapAPIKeyToModel(key *openapi.ApiKey, model *apiKeyModel) {
	model.ID = types.StringValue(key.Id.String())
	model.Name = types.StringValue(key.Name)
	model.OrgID = nullableUUIDToTF(key.OrgId)
	model.Created = nullableTimeToTF(key.Created, model.Created)
	model.PreviewName = types.StringValue(key.PreviewName)
	model.UserID = nullableUUIDToTF(key.UserId)
	model.UserEmail = nullableStringToTF(key.UserEmail)
	model.UserGivenName = nullableStringToTF(key.UserGivenName)
	model.UserFamilyName = nullableStringToTF(key.UserFamilyName)
	model.ExpiresAt = nullableTimeToTF(key.ExpiresAt, model.ExpiresAt)
}

func getAPIKey(ctx context.Context, c *client.Client, id string) (*openapi.ApiKey, error) {
	resp, err := c.API.GetApiKeyIdWithResponse(ctx, id)
	if err != nil {
		return nil, err
	}
	return client.JSON(resp.JSON200)
}

func listAPIKeysByName(ctx context.Context, c *client.Client, name string) ([]openapi.ApiKey, error) {
	return client.ListObjects("/v1/api_key", func(limit int, startingAfter *string) (*client.Page[openapi.ApiKey], error) {
		resp, err := c.API.GetApiKeyWithResponse(ctx, &openapi.GetApiKeyParams{
			Limit: &limit, StartingAfter: startingAfter, ApiKeyName: &name,
		})
		if err != nil {
			return nil, err
		}
		return resp.JSON200, nil
	}, func(key openapi.ApiKey) string { return key.Id.String() })
}

// Filter locally: the API does not promise to honor an org_id query parameter.
// Without an explicit organization, conservatively reject every visible match.
func apiKeyMatches(key openapi.ApiKey, name string, orgID *string) bool {
	return key.Name == name && (orgID == nil || nullableUUIDToTF(key.OrgId).ValueString() == *orgID)
}

// API bodies, transport errors, and decoding errors may contain credentials.
func apiKeyError(err error) string {
	var apiErr *client.APIError
	if errors.As(err, &apiErr) {
		return fmt.Sprintf("Braintrust API returned HTTP %d; response details are omitted to protect credentials.", apiErr.StatusCode)
	}
	if errors.Is(err, context.Canceled) {
		return "The request was canceled."
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "The request deadline was exceeded."
	}
	if client.IsUncertainWriteError(err) {
		return "The write response was unavailable, incomplete, or could not be decoded; response details are omitted to protect credentials."
	}
	return "The API request failed or its response could not be decoded; response details are omitted to protect credentials."
}
