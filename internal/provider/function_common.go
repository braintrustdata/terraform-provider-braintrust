package provider

import (
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/braintrustdata/terraform-provider-braintrust/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type functionModel struct {
	ID                 types.String `tfsdk:"id"`
	XactID             types.String `tfsdk:"xact_id"`
	ProjectID          types.String `tfsdk:"project_id"`
	OrgID              types.String `tfsdk:"org_id"`
	Name               types.String `tfsdk:"name"`
	Slug               types.String `tfsdk:"slug"`
	Description        types.String `tfsdk:"description"`
	Created            types.String `tfsdk:"created"`
	Tags               types.List   `tfsdk:"tags"`
	FunctionType       types.String `tfsdk:"function_type"`
	FunctionDataJSON   types.String `tfsdk:"function_data_json"`
	PromptDataJSON     types.String `tfsdk:"prompt_data_json"`
	FunctionSchemaJSON types.String `tfsdk:"function_schema_json"`
	MetadataJSON       types.String `tfsdk:"metadata_json"`
}

func mapFunctionToModel(function *client.Function, model *functionModel) {
	model.ID = types.StringValue(function.ID)
	model.XactID = types.StringValue(function.XactID)
	model.ProjectID = types.StringValue(function.ProjectID)
	model.OrgID = types.StringValue(function.OrgID)
	model.Name = types.StringValue(function.Name)
	model.Slug = types.StringValue(function.Slug)
	model.Description = stringPtrToTFValue(function.Description)
	model.Created = stringPtrToTFValue(function.Created)
	model.Tags = stringSliceToTFList(function.Tags)
	model.FunctionType = stringPtrToTFValue(function.FunctionType)
	model.FunctionDataJSON = functionJSONState(function.FunctionData, model.FunctionDataJSON)
	model.PromptDataJSON = optionalFunctionJSONState(function.PromptData, model.PromptDataJSON)
	model.FunctionSchemaJSON = optionalFunctionJSONState(function.FunctionSchema, model.FunctionSchemaJSON)
	model.MetadataJSON = optionalFunctionJSONState(function.Metadata, model.MetadataJSON)
}

func optionalFunctionJSONState(raw json.RawMessage, existing types.String) types.String {
	if len(raw) == 0 || string(raw) == "null" {
		return types.StringNull()
	}
	return jsonState(raw, existing)
}

func functionJSONObject(value types.String, field string) (any, error) {
	if value.IsNull() || value.IsUnknown() {
		return nil, nil
	}
	var object map[string]any
	if err := json.Unmarshal([]byte(value.ValueString()), &object); err != nil || object == nil {
		return nil, fmt.Errorf("%s must be a JSON object", field)
	}
	return object, nil
}

// The API may add generated metadata such as a code hash to function_data.
// Keep the configured spelling when all configured fields still match.
func functionJSONState(remote json.RawMessage, existing types.String) types.String {
	if !existing.IsNull() && !existing.IsUnknown() && len(remote) > 0 {
		var planned, actual any
		if json.Unmarshal([]byte(existing.ValueString()), &planned) == nil && json.Unmarshal(remote, &actual) == nil && jsonSubset(planned, actual) {
			return existing
		}
	}
	return jsonRawToTFString(remote)
}

func jsonSubset(planned, actual any) bool {
	plannedMap, ok := planned.(map[string]any)
	if !ok {
		return reflect.DeepEqual(planned, actual)
	}
	actualMap, ok := actual.(map[string]any)
	if !ok {
		return false
	}
	for key, value := range plannedMap {
		actualValue, exists := actualMap[key]
		if !exists || !jsonSubset(value, actualValue) {
			return false
		}
	}
	return true
}
