package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/braintrustdata/terraform-provider-braintrust/internal/client"
)

// Ensure the implementation satisfies the expected interfaces.
var (
	_ datasource.DataSource              = &projectDataSource{}
	_ datasource.DataSourceWithConfigure = &projectDataSource{}
)

// NewProjectDataSource returns a new project data source.
func NewProjectDataSource() datasource.DataSource {
	return &projectDataSource{}
}

// projectDataSource is the data source implementation.
type projectDataSource struct {
	client *client.Client
}

// Metadata returns the data source type name.
func (d *projectDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_project"
}

// Schema defines the schema for the data source.
func (d *projectDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description:         "Use this data source to get information about an existing Braintrust project.",
		MarkdownDescription: "Use this data source to get information about an existing Braintrust [project](https://www.braintrustdata.com/docs/guides/projects).",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Required:    true,
				Description: "The unique identifier (UUID) of the project to look up.",
			},
			"org_id": schema.StringAttribute{
				Computed:    true,
				Description: "Unique identifier for the organization that the project belongs to.",
			},
			"name": schema.StringAttribute{
				Computed:    true,
				Description: "Name of the project.",
			},
			"description": schema.StringAttribute{
				Computed:    true,
				Description: "Textual description of the project.",
			},
			"created": schema.StringAttribute{
				Computed:    true,
				Description: "Date of project creation.",
			},
			"deleted_at": schema.StringAttribute{
				Computed:    true,
				Description: "Date of project deletion, or null if the project is still active.",
			},
			"user_id": schema.StringAttribute{
				Computed:    true,
				Description: "Identifies the user who created the project.",
			},
			"comparison_key": schema.StringAttribute{
				Computed:            true,
				Description:         "The key used to join two experiments (defaults to 'input').",
				MarkdownDescription: "The key used to join two experiments (defaults to `input`).",
			},
			"baseline_experiment_id": schema.StringAttribute{
				Computed:    true,
				Description: "The ID of the experiment to use as the default baseline for comparisons.",
			},
		},
	}
}

// Configure adds the provider configured client to the data source.
func (d *projectDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = clientFromProviderData(req.ProviderData, &resp.Diagnostics)
}

// Read refreshes the Terraform state with the latest data.
func (d *projectDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config projectModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !requireClient(d.client, &resp.Diagnostics) {
		return
	}

	tflog.Debug(ctx, "Reading project data source", map[string]any{
		"id": config.ID.ValueString(),
	})

	project, err := d.client.GetProject(ctx, config.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Reading Braintrust Project",
			"Could not read project ID "+config.ID.ValueString()+": "+err.Error(),
		)
		return
	}

	// Map response to the shared model.
	var state projectModel
	mapProjectToModel(project, &state)

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
