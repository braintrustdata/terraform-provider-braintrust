package provider

import (
	"context"

	"github.com/braintrustdata/terraform-provider-braintrust/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

var (
	_ datasource.DataSource              = &groupDataSource{}
	_ datasource.DataSourceWithConfigure = &groupDataSource{}
)

// NewGroupDataSource returns a new group data source.
func NewGroupDataSource() datasource.DataSource {
	return &groupDataSource{}
}

type groupDataSource struct {
	client *client.Client
}

func (d *groupDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_group"
}

func (d *groupDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description:         "Use this data source to look up a Braintrust group by name. Useful for referencing built-in system groups (Owners, Engineers, Viewers).",
		MarkdownDescription: "Use this data source to look up a Braintrust group by name. Useful for referencing built-in system groups (`Owners`, `Engineers`, `Viewers`).",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "Unique identifier for the group (UUID).",
			},
			"name": schema.StringAttribute{
				Required:    true,
				Description: "Name of the group to look up.",
			},
			"org_id": schema.StringAttribute{
				Computed:    true,
				Description: "Organization the group belongs to.",
			},
			"description": schema.StringAttribute{
				Computed:    true,
				Description: "Textual description of the group.",
			},
			"created": schema.StringAttribute{
				Computed:    true,
				Description: "Date of group creation.",
			},
			"deleted_at": schema.StringAttribute{
				Computed:    true,
				Description: "Date of group deletion, or null if the group is still active.",
			},
			"user_id": schema.StringAttribute{
				Computed:    true,
				Description: "Identifies the user who created the group.",
			},
			"member_users": schema.SetAttribute{
				Computed:    true,
				Description: "IDs of users in this group.",
				ElementType: types.StringType,
			},
			"member_groups": schema.SetAttribute{
				Computed:    true,
				Description: "IDs of groups this group inherits from.",
				ElementType: types.StringType,
			},
		},
	}
}

func (d *groupDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = clientFromProviderData(req.ProviderData, &resp.Diagnostics)
}

func (d *groupDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config groupModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !requireClient(d.client, &resp.Diagnostics) {
		return
	}

	name := config.Name.ValueString()
	tflog.Debug(ctx, "Looking up group by name", map[string]any{
		"name": name,
	})

	groups, err := d.client.ListGroups(ctx, name)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Reading Braintrust Group",
			"Could not list groups: "+err.Error(),
		)
		return
	}

	if len(groups) == 0 {
		resp.Diagnostics.AddError(
			"Group Not Found",
			"No group found with name: "+name,
		)
		return
	}

	var state groupModel
	mapGroupToModel(&groups[0], &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
