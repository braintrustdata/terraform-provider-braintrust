package provider

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/braintrustdata/braintrust-sdk-go/openapi"
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

// dataset keeps metadata as raw JSON: the SDK decodes it into map[string]any,
// which would round large integers through float64.
type dataset = client.Raw[openapi.Dataset]

// mapDatasetToModel maps a dataset to the shared Terraform model.
func mapDatasetToModel(raw *dataset, model *datasetModel) {
	ds := raw.Value
	model.ID = types.StringValue(ds.Id.String())
	model.ProjectID = types.StringValue(ds.ProjectId.String())
	model.Name = types.StringValue(ds.Name)
	model.Description = nullableStringToTF(ds.Description)
	model.Tags = stringSliceToTFList(nullableSliceValue(ds.Tags))
	model.MetadataJSON = jsonState(raw.Fields["metadata"], model.MetadataJSON)
	model.URLSlug = types.StringNull()
	if ds.UrlSlug != "" {
		model.URLSlug = types.StringValue(ds.UrlSlug)
	}
	model.Created = nullableTimeToTF(ds.Created, model.Created)
	model.DeletedAt = nullableTimeToTF(ds.DeletedAt, model.DeletedAt)
	model.UserID = nullableUUIDToTF(ds.UserId)
}

func getDataset(ctx context.Context, c *client.Client, id string) (*dataset, error) {
	resp, err := c.API.GetDatasetIdWithResponse(ctx, id)
	if err != nil {
		return nil, err
	}
	return client.DecodeRaw(resp.JSON200, resp.Body)
}

// listDatasets lists a project's datasets by name, keeping each object's raw JSON.
func listDatasets(ctx context.Context, c *client.Client, name, projectID string) ([]dataset, error) {
	return client.ListAll("/v1/dataset", func(limit int, startingAfter *string) ([]dataset, error) {
		params := &openapi.GetDatasetParams{Limit: &limit, StartingAfter: startingAfter}
		if name != "" {
			params.DatasetName = &name
		}
		if projectID != "" {
			params.ProjectId = &projectID
		}
		page, err := c.API.GetDatasetWithResponse(ctx, params)
		if err != nil {
			return nil, err
		}
		body, err := client.JSON(page.JSON200)
		if err != nil {
			return nil, err
		}
		return client.RawObjects(body.Objects, page.Body)
	}, func(ds dataset) string { return ds.Value.Id.String() })
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
