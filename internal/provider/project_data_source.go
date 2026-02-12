package provider

import (
	"context"

	"github.com/braintrustdata/terraform-provider-braintrust/internal/client"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-log/tflog"
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
		MarkdownDescription: "Use this data source to get information about an existing Braintrust [project](https://www.braintrustdata.com/docs/guides/projects). Look up by `id` or `name`.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "The unique identifier (UUID) of the project. Provide either `id` or `name`.",
				Validators: []validator.String{
					stringvalidator.ExactlyOneOf(path.MatchRoot("name")),
				},
			},
			"name": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "The name of the project. Provide either `name` or `id`.",
			},
			"org_id": schema.StringAttribute{
				Computed:    true,
				Description: "Unique identifier for the organization that the project belongs to.",
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

	var project *client.Project
	var err error

	if !config.Name.IsNull() && !config.Name.IsUnknown() {
		// Look up by name.
		name := config.Name.ValueString()
		tflog.Debug(ctx, "Looking up project by name", map[string]any{"name": name})

		projects, listErr := d.client.ListProjects(ctx, name)
		if listErr != nil {
			resp.Diagnostics.AddError(
				"Error Reading Braintrust Project",
				"Could not list projects: "+listErr.Error(),
			)
			return
		}
		if len(projects) == 0 {
			resp.Diagnostics.AddError(
				"Project Not Found",
				"No project found with name: "+name,
			)
			return
		}
		project = &projects[0]
	} else {
		// Look up by ID.
		id := config.ID.ValueString()
		tflog.Debug(ctx, "Reading project data source", map[string]any{"id": id})

		project, err = d.client.GetProject(ctx, id)
		if err != nil {
			resp.Diagnostics.AddError(
				"Error Reading Braintrust Project",
				"Could not read project ID "+id+": "+err.Error(),
			)
			return
		}
	}

	// Map response to the shared model.
	var state projectModel
	mapProjectToModel(project, &state)

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
