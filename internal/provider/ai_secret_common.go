package provider

import (
	"bytes"
	"context"
	"encoding/json"

	"github.com/braintrustdata/braintrust-sdk-go/openapi"
	"github.com/braintrustdata/terraform-provider-braintrust/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type aiSecret = client.Raw[openapi.AISecret]

// aiSecretModel is the Terraform data model for the AI secret resource.
type aiSecretModel struct {
	Metadata      types.String `tfsdk:"metadata"`
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
func mapAISecretToModel(raw *aiSecret, model *aiSecretModel) {
	secret := raw.Value
	model.Metadata = aiSecretMetadataState(raw.Fields["metadata"], model.Metadata)
	model.ID = types.StringValue(secret.Id.String())
	model.Name = types.StringValue(secret.Name)
	model.OrgID = types.StringValue(secret.OrgId.String())
	model.Type = nullableStringToTF(secret.Type)
	model.PreviewSecret = nullableStringToTF(secret.PreviewSecret)
	model.Created = nullableTimeToTF(secret.Created, model.Created)
	model.UpdatedAt = nullableTimeToTF(secret.UpdatedAt, model.UpdatedAt)
	// model.Secret is intentionally NOT set -- write-only field.
}

func getAISecret(ctx context.Context, c *client.Client, id string) (*aiSecret, error) {
	resp, err := c.API.GetAiSecretIdWithResponse(ctx, id)
	if err != nil {
		return nil, err
	}
	return client.DecodeRaw(resp.JSON200, resp.Body)
}

// jsonb_recursive_merge returns SQL NULL when both inputs have no keys.
// Preserve an explicitly configured empty object in that case. Nonempty
// metadata is always read back, so unsupported clearing cannot be hidden.
func aiSecretMetadataState(raw json.RawMessage, existing types.String) types.String {
	if len(raw) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		if !existing.IsNull() && !existing.IsUnknown() {
			var object map[string]json.RawMessage
			if json.Unmarshal([]byte(existing.ValueString()), &object) == nil && object != nil && len(object) == 0 {
				return existing
			}
		}
	}
	return optionalFunctionJSONState(raw, existing)
}
