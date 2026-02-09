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
	_ datasource.DataSource              = &roleDataSource{}
	_ datasource.DataSourceWithConfigure = &roleDataSource{}
)

// NewRoleDataSource returns a new role data source.
func NewRoleDataSource() datasource.DataSource {
	return &roleDataSource{}
}

type roleDataSource struct {
	client *client.Client
}

func (d *roleDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_role"
}

func (d *roleDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description:         "Use this data source to look up a Braintrust role by name. Useful for referencing built-in system roles (Viewer, Engineer, Owner) in ACL definitions.",
		MarkdownDescription: "Use this data source to look up a Braintrust role by name. Useful for referencing built-in system roles (`Viewer`, `Engineer`, `Owner`) in ACL definitions.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "Unique identifier for the role (UUID).",
			},
			"name": schema.StringAttribute{
				Required:    true,
				Description: "Name of the role to look up.",
			},
			"org_id": schema.StringAttribute{
				Computed:    true,
				Description: "Organization the role belongs to. Null for system roles.",
			},
			"description": schema.StringAttribute{
				Computed:    true,
				Description: "Textual description of the role.",
			},
			"created": schema.StringAttribute{
				Computed:    true,
				Description: "Date of role creation.",
			},
			"deleted_at": schema.StringAttribute{
				Computed:    true,
				Description: "Date of role deletion, or null if the role is still active.",
			},
			"user_id": schema.StringAttribute{
				Computed:    true,
				Description: "Identifies the user who created the role.",
			},
			"member_permissions": schema.ListNestedAttribute{
				Computed:    true,
				Description: "Permissions that belong to this role.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"permission": schema.StringAttribute{
							Computed:    true,
							Description: "The permission type.",
						},
						"restrict_object_type": schema.StringAttribute{
							Computed:    true,
							Description: "Optionally restricts the permission to a specific object type.",
						},
					},
				},
			},
			"member_roles": schema.ListAttribute{
				Computed:    true,
				Description: "IDs of roles this role inherits from.",
				ElementType: types.StringType,
			},
		},
	}
}

func (d *roleDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = clientFromProviderData(req.ProviderData, &resp.Diagnostics)
}

func (d *roleDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config roleModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !requireClient(d.client, &resp.Diagnostics) {
		return
	}

	name := config.Name.ValueString()
	tflog.Debug(ctx, "Looking up role by name", map[string]any{
		"name": name,
	})

	roles, err := d.client.ListRoles(ctx, name)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Reading Braintrust Role",
			"Could not list roles: "+err.Error(),
		)
		return
	}

	if len(roles) == 0 {
		resp.Diagnostics.AddError(
			"Role Not Found",
			"No role found with name: "+name,
		)
		return
	}

	// Use the first matching role.
	var state roleModel
	mapRoleToModel(&roles[0], &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
