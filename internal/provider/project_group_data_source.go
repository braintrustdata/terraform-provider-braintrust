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
		Description: "Looks up a Braintrust project group by organization ID and name.",
		Attributes: map[string]schema.Attribute{
			"id":              schema.StringAttribute{Computed: true},
			"org_id":          schema.StringAttribute{Required: true},
			"name":            schema.StringAttribute{Required: true},
			"description":     schema.StringAttribute{Computed: true},
			"created":         schema.StringAttribute{Computed: true},
			"deleted_at":      schema.StringAttribute{Computed: true},
			"user_id":         schema.StringAttribute{Computed: true},
			"member_projects": schema.SetAttribute{Computed: true, ElementType: types.StringType},
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
