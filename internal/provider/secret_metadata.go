package provider

import (
	"encoding/json"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// secretMetadataAttribute is the metadata attribute shared by AI secrets and
// env vars. It is Optional+Computed so that omitting it leaves metadata
// configured elsewhere (e.g. the UI) alone instead of planning to clear it.
func secretMetadataAttribute(secretField string) schema.StringAttribute {
	return schema.StringAttribute{
		Optional:  true,
		Computed:  true,
		Sensitive: true,
		Description: "Provider-specific configuration as a JSON object. Use jsonencode({...}), file(\"provider.json\"), or jsonencode(yamldecode(file(\"provider.yaml\"))). " +
			"Updates replace the entire object. Omitting this attribute leaves the remote metadata unmanaged; set it to jsonencode({}) to clear it. " +
			"Stored in Terraform state, even though marked sensitive; put the provider key in " + secretField + " instead.",
		PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
	}
}

// secretMetadataState maps remote metadata to state. The API may report an
// empty object as null, {}, or omit it; keep Terraform's existing null/empty
// spelling for those without hiding nonempty drift.
func secretMetadataState(raw json.RawMessage, existing types.String) types.String {
	if isEmptyJSONObject(raw) {
		if !existing.IsUnknown() && (existing.IsNull() || isEmptyJSONObject(json.RawMessage(existing.ValueString()))) {
			return existing
		}
		return types.StringNull()
	}
	return jsonState(raw, existing)
}

func isEmptyJSONObject(raw json.RawMessage) bool {
	if len(raw) == 0 {
		return true
	}
	value, err := decodeJSONValue(raw)
	if err != nil {
		return false
	}
	if value == nil {
		return true
	}
	object, ok := value.(map[string]any)
	return ok && len(object) == 0
}

// secretMetadataPatch returns the metadata to send on update, or ok=false
// when it is unchanged or not configured (unknown).
func secretMetadataPatch(plan, state types.String) (metadata json.RawMessage, ok bool, err error) {
	if plan.IsUnknown() || plan.IsNull() || scoreJSONEqual(plan, state) {
		return nil, false, nil
	}
	metadata, err = functionJSONObject(plan, "metadata")
	return metadata, err == nil, err
}

// checkSecretMetadataApplied reports an error when the API did not store the
// planned metadata (for example, an API that merges instead of replacing).
// The caller still saves the server's value so the next plan shows the drift.
func checkSecretMetadataApplied(planned, stored types.String, diags *diag.Diagnostics) {
	if planned.IsUnknown() || planned.Equal(stored) {
		return
	}
	diags.AddAttributeError(path.Root("metadata"), "Metadata Not Applied",
		"The Braintrust API stored different metadata than planned. The API may not support replacing metadata; "+
			"upgrade the Braintrust API or update the metadata in the UI. State records the value the API returned.")
}
