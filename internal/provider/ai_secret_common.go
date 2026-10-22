package provider

import (
	"context"

	"github.com/braintrustdata/braintrust-sdk-go/openapi"
	"github.com/braintrustdata/terraform-provider-braintrust/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// aiSecretModel is the Terraform data model for the AI secret resource.
type aiSecretModel struct {
	ID            types.String `tfsdk:"id"`
	Name          types.String `tfsdk:"name"`
	Secret        types.String `tfsdk:"secret"`
	Type          types.String `tfsdk:"type"`
	OrgID         types.String `tfsdk:"org_id"`
	PreviewSecret types.String `tfsdk:"preview_secret"`
	Created       types.String `tfsdk:"created"`
	UpdatedAt     types.String `tfsdk:"updated_at"`
}

// mapAISecretToModel maps an SDK AI secret (GET response) to the Terraform model.
// Note: Secret is NOT set here because the GET response doesn't include it.
// The raw secret is only sent during create/update, never returned.
func mapAISecretToModel(secret *openapi.AISecret, model *aiSecretModel) {
	model.ID = types.StringValue(secret.Id.String())
	model.Name = types.StringValue(secret.Name)
	model.OrgID = types.StringValue(secret.OrgId.String())
	model.Type = nullableStringToTF(secret.Type)
	model.PreviewSecret = nullableStringToTF(secret.PreviewSecret)
	model.Created = nullableTimeToTF(secret.Created, model.Created)
	model.UpdatedAt = nullableTimeToTF(secret.UpdatedAt, model.UpdatedAt)
	// model.Secret is intentionally NOT set -- write-only field.
}

func getAISecret(ctx context.Context, c *client.Client, id string) (*openapi.AISecret, error) {
	resp, err := c.API.GetAiSecretIdWithResponse(ctx, id)
	if err != nil {
		return nil, err
	}
	return client.JSON(resp.JSON200)
}
