package provider

import (
	"context"

	"github.com/braintrustdata/terraform-provider-braintrust/internal/client"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

var (
	_ datasource.DataSource              = &datasetDataSource{}
	_ datasource.DataSourceWithConfigure = &datasetDataSource{}
)

// NewDatasetDataSource returns a new dataset data source.
func NewDatasetDataSource() datasource.DataSource {
	return &datasetDataSource{}
}

type datasetDataSource struct {
	client *client.Client
}

func (d *datasetDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_dataset"
}

func (d *datasetDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description:         "Use this data source to get information about an existing Braintrust dataset.",
		MarkdownDescription: "Use this data source to get information about an existing Braintrust [dataset](https://www.braintrustdata.com/docs/guides/datasets). Look up by `id` or by `name` + `project_id`.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "The unique identifier (UUID) of the dataset. Provide either `id` or `name` (with `project_id`).",
				Validators: []validator.String{
					stringvalidator.ExactlyOneOf(path.MatchRoot("name")),
				},
			},
			"project_id": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "The project the dataset belongs to. Required when looking up by `name`.",
			},
			"name": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "The name of the dataset. Provide `name` together with `project_id`, or use `id` instead.",
			},
			"description": schema.StringAttribute{
				Computed:    true,
				Description: "Textual description of the dataset.",
			},
			"tags":          schema.ListAttribute{Computed: true, ElementType: types.StringType, Description: "Dataset tags."},
			"metadata_json": schema.StringAttribute{Computed: true, Description: "JSON-encoded dataset metadata object."},
			"url_slug":      schema.StringAttribute{Computed: true, Description: "URL slug assigned to the dataset by Braintrust."},
			"created": schema.StringAttribute{
				Computed:    true,
				Description: "Date of dataset creation.",
			},
			"deleted_at": schema.StringAttribute{
				Computed:    true,
				Description: "Date of dataset deletion, or null if the dataset is still active.",
			},
			"user_id": schema.StringAttribute{
				Computed:    true,
				Description: "Identifies the user who created the dataset.",
			},
		},
	}
}

func (d *datasetDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = clientFromProviderData(req.ProviderData, &resp.Diagnostics)
}

func (d *datasetDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config datasetModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !requireClient(d.client, &resp.Diagnostics) {
		return
	}

	var dataset *client.Dataset
	var err error

	if !config.Name.IsNull() && !config.Name.IsUnknown() {
		// Look up by name — project_id is required for scoping.
		name := config.Name.ValueString()

		if config.ProjectID.IsNull() || config.ProjectID.IsUnknown() {
			resp.Diagnostics.AddError(
				"Missing Required Attribute",
				"\"project_id\" is required when looking up a dataset by name.",
			)
			return
		}
		projectID := config.ProjectID.ValueString()

		tflog.Debug(ctx, "Looking up dataset by name", map[string]any{
			"name":       name,
			"project_id": projectID,
		})

		datasets, listErr := d.client.ListDatasets(ctx, name, projectID)
		if listErr != nil {
			resp.Diagnostics.AddError(
				"Error Reading Braintrust Dataset",
				"Could not list datasets: "+listErr.Error(),
			)
			return
		}
		var matches []client.Dataset
		for _, candidate := range datasets {
			if candidate.Name == name && candidate.ProjectID == projectID && candidate.DeletedAt == nil {
				matches = append(matches, candidate)
			}
		}
		if len(matches) == 0 {
			resp.Diagnostics.AddError(
				"Dataset Not Found",
				"No active dataset found with name "+name+" in project "+projectID+".",
			)
			return
		}
		if len(matches) > 1 {
			resp.Diagnostics.AddError("Ambiguous Dataset", "Multiple active datasets have the name "+name+" in project "+projectID+". Look up the dataset by ID.")
			return
		}
		dataset = &matches[0]
	} else {
		// Look up by ID.
		id := config.ID.ValueString()
		tflog.Debug(ctx, "Reading dataset data source", map[string]any{"id": id})

		dataset, err = d.client.GetDataset(ctx, id)
		if err != nil {
			resp.Diagnostics.AddError(
				"Error Reading Braintrust Dataset",
				"Could not read dataset ID "+id+": "+err.Error(),
			)
			return
		}
		if !config.ProjectID.IsNull() && !config.ProjectID.IsUnknown() && dataset.ProjectID != config.ProjectID.ValueString() {
			resp.Diagnostics.AddError("Dataset Project Mismatch", "Dataset ID "+id+" does not belong to project_id "+config.ProjectID.ValueString()+".")
			return
		}
	}

	var state datasetModel
	mapDatasetToModel(dataset, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
