package provider

import (
	"encoding/json"
	"fmt"

	"github.com/braintrustdata/terraform-provider-braintrust/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// datasetModel is the shared data model for both the dataset resource and data source.
type datasetModel struct {
	ID           types.String `tfsdk:"id"`
	ProjectID    types.String `tfsdk:"project_id"`
	Name         types.String `tfsdk:"name"`
	Description  types.String `tfsdk:"description"`
	Tags         types.List   `tfsdk:"tags"`
	MetadataJSON types.String `tfsdk:"metadata_json"`
	URLSlug      types.String `tfsdk:"url_slug"`
	Created      types.String `tfsdk:"created"`
	DeletedAt    types.String `tfsdk:"deleted_at"`
	UserID       types.String `tfsdk:"user_id"`
}

// mapDatasetToModel maps a client.Dataset to the shared Terraform model.
func mapDatasetToModel(dataset *client.Dataset, model *datasetModel) {
	model.ID = types.StringValue(dataset.ID)
	model.ProjectID = types.StringValue(dataset.ProjectID)
	model.Name = types.StringValue(dataset.Name)
	model.Description = stringPtrToTFValue(dataset.Description)
	model.Tags = stringSliceToTFList(dataset.Tags)
	model.MetadataJSON = jsonState(dataset.Metadata, model.MetadataJSON)
	model.URLSlug = stringPtrToTFValue(dataset.URLSlug)
	model.Created = stringPtrToTFValue(dataset.Created)
	model.DeletedAt = stringPtrToTFValue(dataset.DeletedAt)
	model.UserID = stringPtrToTFValue(dataset.UserID)
}

func datasetMetadataInput(value types.String) (json.RawMessage, error) {
	if value.IsNull() || value.IsUnknown() {
		return nil, nil
	}
	raw := json.RawMessage(value.ValueString())
	var decoded any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return nil, fmt.Errorf("metadata_json must be valid JSON: %w", err)
	}
	if decoded != nil {
		if _, ok := decoded.(map[string]any); !ok {
			return nil, fmt.Errorf("metadata_json must be a JSON object or null")
		}
	}
	return raw, nil
}
