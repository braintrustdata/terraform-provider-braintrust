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
	_ datasource.DataSource              = &promptDataSource{}
	_ datasource.DataSourceWithConfigure = &promptDataSource{}
)

// NewPromptDataSource returns a new prompt data source.
func NewPromptDataSource() datasource.DataSource {
	return &promptDataSource{}
}

type promptDataSource struct {
	client *client.Client
}

func (d *promptDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_prompt"
}

func (d *promptDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description:         "Use this data source to get information about an existing Braintrust prompt.",
		MarkdownDescription: "Use this data source to get information about an existing Braintrust prompt. Look up by `id` or by `name` + `project_id`.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "The unique identifier (UUID) of the prompt. Provide either `id` or `name` (with `project_id`).",
				Validators: []validator.String{
					stringvalidator.ExactlyOneOf(path.MatchRoot("name")),
				},
			},
			"project_id": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "The project the prompt belongs to. Required when looking up by `name`.",
			},
			"name": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "The name of the prompt. Provide `name` together with `project_id`, or use `id` instead.",
			},
			"slug": schema.StringAttribute{
				Computed:    true,
				Description: "Slug/identifier of the prompt.",
			},
			"description": schema.StringAttribute{
				Computed:    true,
				Description: "Textual description of the prompt.",
			},
			"created": schema.StringAttribute{
				Computed:    true,
				Description: "Date of prompt creation.",
			},
			"xact_id": schema.StringAttribute{
				Computed:    true,
				Description: "Transaction id of the prompt version.",
			},
			"org_id": schema.StringAttribute{
				Computed:    true,
				Description: "Unique identifier for the organization.",
			},
			"function_type": schema.StringAttribute{
				Computed:    true,
				Description: "Optional function type for the prompt.",
			},
			"prompt_data_json": schema.StringAttribute{
				Computed:    true,
				Description: "JSON-encoded prompt_data payload.",
			},
			"metadata_json": schema.StringAttribute{
				Computed:    true,
				Description: "JSON-encoded metadata object returned by the API.",
			},
			"tags": schema.ListAttribute{
				Computed:    true,
				ElementType: types.StringType,
				Description: "A list of tags for the prompt.",
			},
		},
	}
}

func (d *promptDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = clientFromProviderData(req.ProviderData, &resp.Diagnostics)
}

func (d *promptDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config promptModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !requireClient(d.client, &resp.Diagnostics) {
		return
	}

	var prompt *client.Prompt
	var err error

	if !config.Name.IsNull() && !config.Name.IsUnknown() {
		name := config.Name.ValueString()

		if config.ProjectID.IsNull() || config.ProjectID.IsUnknown() {
			resp.Diagnostics.AddError(
				"Missing Required Attribute",
				"\"project_id\" is required when looking up a prompt by name.",
			)
			return
		}
		projectID := config.ProjectID.ValueString()

		tflog.Debug(ctx, "Looking up prompt by name", map[string]any{
			"name":       name,
			"project_id": projectID,
		})

		prompts, listErr := d.client.ListPrompts(ctx, name, projectID)
		if listErr != nil {
			resp.Diagnostics.AddError(
				"Error Reading Braintrust Prompt",
				"Could not list prompts: "+listErr.Error(),
			)
			return
		}
		if len(prompts) == 0 {
			resp.Diagnostics.AddError(
				"Prompt Not Found",
				"No prompt found with name: "+name,
			)
			return
		}
		prompt = &prompts[0]
	} else {
		id := config.ID.ValueString()
		tflog.Debug(ctx, "Reading prompt data source", map[string]any{"id": id})

		prompt, err = d.client.GetPrompt(ctx, id)
		if err != nil {
			resp.Diagnostics.AddError(
				"Error Reading Braintrust Prompt",
				"Could not read prompt ID "+id+": "+err.Error(),
			)
			return
		}
	}

	var state promptModel
	mapPromptToModel(prompt, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
