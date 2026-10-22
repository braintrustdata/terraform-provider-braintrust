package provider

import (
	"context"

	"github.com/braintrustdata/braintrust-sdk-go/openapi"
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
		Description:         "Look up a Braintrust group by name and optional organization ID. Useful for referencing built-in groups (Owners, Engineers, Viewers, All AI Provider Access).",
		MarkdownDescription: "Look up a Braintrust group by name and optional organization ID. Useful for referencing built-in groups (`Owners`, `Engineers`, `Viewers`, `All AI Provider Access`).",
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
				Optional:    true,
				Computed:    true,
				Description: "Organization the group belongs to. Set this when the credential can access multiple organizations.",
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
	orgID := config.OrgID.ValueString()
	tflog.Debug(ctx, "Looking up group by name", map[string]any{
		"name": name, "org_id": orgID,
	})

	groups, err := listGroups(ctx, d.client, name, orgID)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Reading Braintrust Group",
			"Could not list groups: "+err.Error(),
		)
		return
	}

	var match *openapi.Group
	for i := range groups {
		group := &groups[i]
		if group.Name == name && !nullableIsSet(group.DeletedAt) && (orgID == "" || group.OrgId.String() == orgID) {
			if match != nil {
				resp.Diagnostics.AddError(
					"Ambiguous Group",
					"Multiple groups have the name "+name+". Set org_id to select one organization.",
				)
				return
			}
			match = group
		}
	}
	if match == nil {
		resp.Diagnostics.AddError(
			"Group Not Found",
			"No group found with name: "+name,
		)
		return
	}
	var state groupModel
	mapGroupToModel(match, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
