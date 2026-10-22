package provider

import (
	"context"

	"github.com/braintrustdata/braintrust-sdk-go/openapi"
	"github.com/braintrustdata/terraform-provider-braintrust/internal/client"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ datasource.DataSource              = &functionDataSource{}
	_ datasource.DataSourceWithConfigure = &functionDataSource{}
)

func NewFunctionDataSource() datasource.DataSource { return &functionDataSource{} }

type functionDataSource struct{ client *client.Client }

func (d *functionDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_function"
}

func (d *functionDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Reads an existing Braintrust function by ID or name and project.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Optional: true, Computed: true,
				Description: "Function ID. Provide either id or name with project_id.",
				Validators:  []validator.String{stringvalidator.ExactlyOneOf(path.MatchRoot("name"))},
			},
			"project_id":           schema.StringAttribute{Optional: true, Computed: true, Description: "Project ID, required for name lookup."},
			"name":                 schema.StringAttribute{Optional: true, Computed: true, Description: "Display name of the function."},
			"slug":                 schema.StringAttribute{Computed: true, Description: "Unique function slug."},
			"description":          schema.StringAttribute{Computed: true, Description: "Textual description."},
			"created":              schema.StringAttribute{Computed: true, Description: "Creation time."},
			"xact_id":              schema.StringAttribute{Computed: true, Description: "Transaction ID of the function version."},
			"org_id":               schema.StringAttribute{Computed: true, Description: "Organization ID."},
			"function_type":        schema.StringAttribute{Computed: true, Description: "Function type."},
			"function_data_json":   schema.StringAttribute{Computed: true, Description: "Function implementation as JSON."},
			"prompt_data_json":     schema.StringAttribute{Computed: true, Description: "Optional prompt configuration as JSON."},
			"function_schema_json": schema.StringAttribute{Computed: true, Description: "Optional parameter and return schema as JSON."},
			"metadata_json":        schema.StringAttribute{Computed: true, Description: "Metadata as JSON."},
			"tags":                 schema.ListAttribute{Computed: true, ElementType: types.StringType, Description: "Function tags."},
		},
	}
}

func (d *functionDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = clientFromProviderData(req.ProviderData, &resp.Diagnostics)
}

func (d *functionDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config functionModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() || !requireClient(d.client, &resp.Diagnostics) {
		return
	}
	var function *function
	if !config.Name.IsNull() && !config.Name.IsUnknown() {
		if config.ProjectID.IsNull() || config.ProjectID.IsUnknown() {
			resp.Diagnostics.AddError("Missing Project ID", "project_id is required when looking up a function by name.")
			return
		}
		name, projectID := config.Name.ValueString(), config.ProjectID.ValueString()
		functions, err := listFunctions(ctx, d.client, name, projectID)
		if err != nil {
			resp.Diagnostics.AddError("Error Listing Braintrust Functions", err.Error())
			return
		}
		var match *openapi.Function
		for i := range functions {
			candidate := &functions[i]
			if candidate.Name == name && candidate.ProjectId.String() == projectID {
				if match != nil {
					resp.Diagnostics.AddError("Ambiguous Function", "Multiple functions have this name in the project. Look up by ID.")
					return
				}
				match = candidate
			}
		}
		if match == nil {
			resp.Diagnostics.AddError("Function Not Found", "No function with this name exists in the project.")
			return
		}
		// Re-read by ID so the JSON attributes are kept as raw JSON.
		function, err = getFunction(ctx, d.client, match.Id.String())
		if err != nil {
			resp.Diagnostics.AddError("Error Reading Braintrust Function", err.Error())
			return
		}
	} else {
		var err error
		function, err = getFunction(ctx, d.client, config.ID.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Error Reading Braintrust Function", err.Error())
			return
		}
		if !config.ProjectID.IsNull() && !config.ProjectID.IsUnknown() && function.Value.ProjectId.String() != config.ProjectID.ValueString() {
			resp.Diagnostics.AddError("Function Project Mismatch", "The function ID does not belong to project_id.")
			return
		}
	}
	var state functionModel
	mapFunctionToModel(function, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
