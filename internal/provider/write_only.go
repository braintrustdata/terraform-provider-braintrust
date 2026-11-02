package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// writeOnlyField describes a write-only value attribute paired with the
// version attribute that triggers sending it.
type writeOnlyField struct {
	valueName, versionName string
	// allowClear permits a configured version with a null value, which clears
	// the remote value.
	allowClear bool
}

var (
	aiSecretWriteOnly = writeOnlyField{valueName: "secret", versionName: "secret_version"}
	envVarWriteOnly   = writeOnlyField{valueName: "value", versionName: "value_version", allowClear: true}
)

// validate rechecks the resolved inputs before applying. Configuration
// validators defer unknown inputs, so an ephemeral value could otherwise bypass
// the version requirement.
func (f writeOnlyField) validate(value types.String, version types.Int64, diagnostics *diag.Diagnostics) {
	valueName, versionName := f.valueName, f.versionName
	if value.IsUnknown() || version.IsUnknown() {
		diagnostics.AddAttributeError(path.Root(valueName), "Unknown Write-Only Input", "The value and its version must be known before applying.")
		return
	}
	if !version.IsNull() && version.ValueInt64() < 1 {
		diagnostics.AddAttributeError(path.Root(versionName), "Invalid Write-Only Version", "The version must be a positive integer.")
	}
	if !value.IsNull() && version.IsNull() {
		diagnostics.AddAttributeError(path.Root(versionName), "Missing Write-Only Version", versionName+" must be configured when "+valueName+" is configured.")
	}
	if !f.allowClear && !version.IsNull() && value.IsNull() {
		diagnostics.AddAttributeError(path.Root(valueName), "Missing Write-Only Value", valueName+" must be configured when "+versionName+" is configured; this API cannot clear the value.")
	}
}

// versionChanged reports whether the planned version differs from state, which
// means the write-only value must be sent.
func (f writeOnlyField) versionChanged(plan, state types.Int64) bool {
	return !plan.IsNull() && !plan.Equal(state)
}

// stateUpgrader discards the formerly persisted value without making any API
// calls. All other attributes retain their exact prior state values.
func (f writeOnlyField) stateUpgrader() resource.StateUpgrader {
	valueName, versionName := f.valueName, f.versionName
	return resource.StateUpgrader{
		StateUpgrader: func(ctx context.Context, req resource.UpgradeStateRequest, resp *resource.UpgradeStateResponse) {
			currentType, ok := resp.State.Schema.Type().TerraformType(ctx).(tftypes.Object)
			if !ok {
				resp.Diagnostics.AddError("Error Upgrading Write-Only State", "Expected an object resource schema. This is a provider bug.")
				return
			}
			priorAttributes := make(map[string]tftypes.Type, len(currentType.AttributeTypes)-1)
			for name, attrType := range currentType.AttributeTypes {
				if name != versionName {
					priorAttributes[name] = attrType
				}
			}
			// Drop attributes the current schema no longer has, as the framework does.
			prior, err := req.RawState.UnmarshalWithOpts(tftypes.Object{AttributeTypes: priorAttributes}, tfprotov6.UnmarshalOpts{
				ValueFromJSONOpts: tftypes.ValueFromJSONOpts{IgnoreUndefinedAttributes: true},
			})
			if err != nil {
				// Do not include raw-state decoding errors: they may contain plaintext.
				resp.Diagnostics.AddError("Error Upgrading Write-Only State", "Could not decode the previous resource state.")
				return
			}
			var attributes map[string]tftypes.Value
			if err := prior.As(&attributes); err != nil {
				resp.Diagnostics.AddError("Error Upgrading Write-Only State", "Could not read the previous resource attributes.")
				return
			}
			attributes[valueName] = tftypes.NewValue(tftypes.String, nil)
			attributes[versionName] = tftypes.NewValue(tftypes.Number, nil)
			resp.State.Raw = tftypes.NewValue(currentType, attributes)
		},
	}
}
