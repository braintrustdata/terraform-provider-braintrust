package provider

import (
	"context"

	"github.com/braintrustdata/terraform-provider-braintrust/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
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
		MarkdownDescription: "Use this data source to get information about an existing Braintrust [dataset](https://www.braintrustdata.com/docs/guides/datasets).",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Required:    true,
				Description: "The unique identifier (UUID) of the dataset to look up.",
			},
			"project_id": schema.StringAttribute{
				Computed:    true,
				Description: "Unique identifier for the project that the dataset belongs to.",
			},
			"name": schema.StringAttribute{
				Computed:    true,
				Description: "Name of the dataset.",
			},
			"description": schema.StringAttribute{
				Computed:    true,
				Description: "Textual description of the dataset.",
			},
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

	tflog.Debug(ctx, "Reading dataset data source", map[string]any{
		"id": config.ID.ValueString(),
	})

	dataset, err := d.client.GetDataset(ctx, config.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Reading Braintrust Dataset",
			"Could not read dataset ID "+config.ID.ValueString()+": "+err.Error(),
		)
		return
	}

	var state datasetModel
	mapDatasetToModel(dataset, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
