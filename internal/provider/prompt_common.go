package provider

import (
	"encoding/json"

	"github.com/braintrustdata/terraform-provider-braintrust/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// promptModel is the Terraform data model for prompt resources and data sources.
type promptModel struct {
	ID             types.String `tfsdk:"id"`
	XactID         types.String `tfsdk:"xact_id"`
	ProjectID      types.String `tfsdk:"project_id"`
	OrgID          types.String `tfsdk:"org_id"`
	Name           types.String `tfsdk:"name"`
	Slug           types.String `tfsdk:"slug"`
	Description    types.String `tfsdk:"description"`
	Created        types.String `tfsdk:"created"`
	Tags           types.List   `tfsdk:"tags"`
	FunctionType   types.String `tfsdk:"function_type"`
	PromptDataJSON types.String `tfsdk:"prompt_data_json"`
	MetadataJSON   types.String `tfsdk:"metadata_json"`
}

func mapPromptToModel(prompt *client.Prompt, model *promptModel) {
	model.ID = types.StringValue(prompt.ID)
	model.XactID = types.StringValue(prompt.XactID)
	model.ProjectID = types.StringValue(prompt.ProjectID)
	model.OrgID = types.StringValue(prompt.OrgID)
	model.Name = types.StringValue(prompt.Name)
	model.Slug = types.StringValue(prompt.Slug)
	model.Description = stringPtrToTFValue(prompt.Description)
	model.Created = stringPtrToTFValue(prompt.Created)
	model.Tags = stringSliceToTFList(prompt.Tags)
	model.FunctionType = stringPtrToTFValue(prompt.FunctionType)
	model.PromptDataJSON = jsonRawToTFString(prompt.PromptData)
	model.MetadataJSON = jsonRawToTFString(prompt.Metadata)
}

func jsonRawToTFString(raw json.RawMessage) types.String {
	if len(raw) == 0 {
		return types.StringNull()
	}
	return types.StringValue(string(raw))
}

func tfJSONStringToAny(s types.String) (any, error) {
	if s.IsNull() || s.IsUnknown() {
		return nil, nil
	}
	var v any
	if err := json.Unmarshal([]byte(s.ValueString()), &v); err != nil {
		return nil, err
	}
	return v, nil
}
