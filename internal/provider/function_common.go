package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/braintrustdata/braintrust-sdk-go/openapi"
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

// function keeps the JSON-encoded attributes as raw JSON, which Terraform
// passes through verbatim instead of round-tripping them through SDK models.
type function = client.Raw[openapi.Function]

func mapFunctionToModel(raw *function, model *functionModel) {
	fn := raw.Value
	model.ID = types.StringValue(fn.Id.String())
	model.XactID = types.StringValue(fn.UnderscoreXactId)
	model.ProjectID = types.StringValue(fn.ProjectId.String())
	model.OrgID = types.StringValue(fn.OrgId.String())
	model.Name = types.StringValue(fn.Name)
	model.Slug = types.StringValue(fn.Slug)
	model.Description = nullableStringToTF(fn.Description)
	model.Created = nullableTimeToTF(fn.Created, model.Created)
	model.Tags = stringSliceToTFList(nullableSliceValue(fn.Tags))
	model.FunctionType = types.StringNull()
	if nullableIsSet(fn.FunctionType) {
		model.FunctionType = types.StringValue(string(fn.FunctionType.MustGet()))
	}
	model.FunctionDataJSON = functionJSONState(raw.Fields["function_data"], model.FunctionDataJSON)
	model.PromptDataJSON = optionalFunctionJSONState(raw.Fields["prompt_data"], model.PromptDataJSON)
	model.FunctionSchemaJSON = optionalFunctionJSONState(raw.Fields["function_schema"], model.FunctionSchemaJSON)
	model.MetadataJSON = optionalFunctionJSONState(raw.Fields["metadata"], model.MetadataJSON)
}

func getFunction(ctx context.Context, c *client.Client, id string) (*function, error) {
	resp, err := c.API.GetFunctionIdWithResponse(ctx, id, nil)
	if err != nil {
		return nil, err
	}
	return client.DecodeRaw(resp.JSON200, resp.Body)
}

// listFunctions lists a project's functions by name, keeping each object's raw JSON.
func listFunctions(ctx context.Context, c *client.Client, name, projectID string) ([]function, error) {
	return client.ListAll("/v1/function", func(limit int, startingAfter *string) ([]function, error) {
		params := &openapi.GetFunctionParams{Limit: &limit, StartingAfter: startingAfter}
		if name != "" {
			params.FunctionName = &name
		}
		if projectID != "" {
			params.ProjectId = &projectID
		}
		page, err := c.API.GetFunctionWithResponse(ctx, params)
		if err != nil {
			return nil, err
		}
		body, err := client.JSON(page.JSON200)
		if err != nil {
			return nil, err
		}
		return client.RawObjects(body.Objects, page.Body)
	}, func(fn function) string { return fn.Value.Id.String() })
}

func optionalFunctionJSONState(raw json.RawMessage, existing types.String) types.String {
	if len(raw) == 0 || string(raw) == "null" {
		return types.StringNull()
	}
	return jsonState(raw, existing)
}

func functionJSONObject(value types.String, field string) (json.RawMessage, error) {
	if value.IsNull() || value.IsUnknown() {
		return nil, nil
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal([]byte(value.ValueString()), &object); err != nil || object == nil {
		return nil, fmt.Errorf("%s must be a JSON object", field)
	}
	return json.RawMessage(value.ValueString()), nil
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
