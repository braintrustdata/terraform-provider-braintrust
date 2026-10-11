package provider

import (
	"encoding/json"
	"reflect"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Preserve the configured JSON spelling when the API returns an equivalent value.
func jsonState(remote json.RawMessage, existing types.String) types.String {
	if !existing.IsNull() && !existing.IsUnknown() && len(remote) > 0 {
		var planned, actual any
		if json.Unmarshal([]byte(existing.ValueString()), &planned) == nil && json.Unmarshal(remote, &actual) == nil && reflect.DeepEqual(planned, actual) {
			return existing
		}
	}
	return jsonRawToTFString(remote)
}

func jsonRawToTFString(raw json.RawMessage) types.String {
	if len(raw) == 0 {
		return types.StringNull()
	}
	var value any
	if json.Unmarshal(raw, &value) == nil {
		if canonical, err := json.Marshal(value); err == nil {
			return types.StringValue(string(canonical))
		}
	}
	return types.StringValue(string(raw))
}
