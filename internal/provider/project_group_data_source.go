package provider

import (
	"context"

	"github.com/braintrustdata/braintrust-sdk-go/openapi"
	"github.com/braintrustdata/terraform-provider-braintrust/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ datasource.DataSource              = &projectGroupDataSource{}
	_ datasource.DataSourceWithConfigure = &projectGroupDataSource{}
)

func NewProjectGroupDataSource() datasource.DataSource { return &projectGroupDataSource{} }

type projectGroupDataSource struct{ client *client.Client }

func (d *projectGroupDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_project_group"
}

func (d *projectGroupDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Use this data source to look up a Braintrust project group by organization ID and name.",
		Attributes: map[string]schema.Attribute{
			"id":              schema.StringAttribute{Computed: true, Description: "Unique identifier for the project group (UUID)."},
			"org_id":          schema.StringAttribute{Required: true, Description: "Organization the project group belongs to."},
			"name":            schema.StringAttribute{Required: true, Description: "Name of the project group to look up."},
			"description":     schema.StringAttribute{Computed: true, Description: "Textual description of the project group."},
			"created":         schema.StringAttribute{Computed: true, Description: "Date of project group creation."},
			"deleted_at":      schema.StringAttribute{Computed: true, Description: "Date of project group deletion, or null if the project group is still active."},
			"user_id":         schema.StringAttribute{Computed: true, Description: "Identifies the user who created the project group."},
			"member_projects": schema.SetAttribute{Computed: true, ElementType: types.StringType, Description: "IDs of projects in this project group."},
		},
	}
}

func (d *projectGroupDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = clientFromProviderData(req.ProviderData, &resp.Diagnostics)
}

func (d *projectGroupDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config projectGroupModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() || !requireClient(d.client, &resp.Diagnostics) {
		return
	}
	groups, err := listProjectGroups(ctx, d.client, config.OrgID.ValueString(), config.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error Reading Project Group", err.Error())
		return
	}
	var match *openapi.ProjectGroup
	for i := range groups {
		if groups[i].OrgId.String() == config.OrgID.ValueString() && groups[i].Name == config.Name.ValueString() && !nullableIsSet(groups[i].DeletedAt) {
			if match != nil {
				resp.Diagnostics.AddError("Ambiguous Project Group", "Multiple active project groups matched the organization ID and name.")
				return
			}
			match = &groups[i]
		}
	}
	if match == nil {
		resp.Diagnostics.AddError("Project Group Not Found", "No active project group matched the organization ID and name.")
		return
	}
	mapProjectGroupToModel(match, &config)
	resp.Diagnostics.Append(resp.State.Set(ctx, config)...)
}
