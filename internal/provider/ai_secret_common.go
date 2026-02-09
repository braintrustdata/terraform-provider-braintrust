package provider

import (
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

// mapAISecretToModel maps a client.AISecret (GET response) to the Terraform model.
// Note: Secret is NOT set here because the GET response doesn't include it.
// The raw secret is only sent during create/update, never returned.
func mapAISecretToModel(secret *client.AISecret, model *aiSecretModel) {
	model.ID = types.StringValue(secret.ID)
	model.Name = types.StringValue(secret.Name)
	model.OrgID = types.StringValue(secret.OrgID)
	model.Type = stringPtrToTFValue(secret.Type)
	model.PreviewSecret = stringPtrToTFValue(secret.PreviewSecret)
	model.Created = stringPtrToTFValue(secret.Created)
	model.UpdatedAt = stringPtrToTFValue(secret.UpdatedAt)
	// model.Secret is intentionally NOT set -- write-only field.
}
