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
		Description:         "Look up a Braintrust role by ID or name. A name lookup fails if multiple roles match; set org_id for a custom role or use the ID of a system role.",
		MarkdownDescription: "Look up a Braintrust role by ID or name. A name lookup fails if multiple roles match; set `org_id` for a custom role or use the ID of a system role.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Unique identifier for the role (UUID). Provide either id or name.",
				Validators:  []validator.String{stringvalidator.ExactlyOneOf(path.MatchRoot("name"))},
			},
			"name": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Name of the role to look up. Provide either name or id.",
			},
			"org_id": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Organization ID to match for a custom role. Null for system roles.",
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
			"member_permissions": schema.SetNestedAttribute{
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
			"member_roles": schema.SetAttribute{
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

	var role *client.Role
	if !config.ID.IsNull() && !config.ID.IsUnknown() {
		id := config.ID.ValueString()
		tflog.Debug(ctx, "Looking up role by ID", map[string]any{"id": id})
		var err error
		role, err = d.client.GetRole(ctx, id)
		if err != nil {
			resp.Diagnostics.AddError("Error Reading Braintrust Role", "Could not read role ID "+id+": "+err.Error())
			return
		}
		if !config.OrgID.IsNull() && !config.OrgID.IsUnknown() && (role.OrgID == nil || *role.OrgID != config.OrgID.ValueString()) {
			resp.Diagnostics.AddError("Role Organization Mismatch", "Role ID "+id+" does not belong to org_id "+config.OrgID.ValueString()+".")
			return
		}
	} else {
		name := config.Name.ValueString()
		tflog.Debug(ctx, "Looking up role by name", map[string]any{"name": name})
		roles, err := d.client.ListRoles(ctx, name)
		if err != nil {
			resp.Diagnostics.AddError("Error Reading Braintrust Role", "Could not list roles: "+err.Error())
			return
		}
		for i := range roles {
			candidate := &roles[i]
			if candidate.Name != name || candidate.DeletedAt != nil {
				continue
			}
			if !config.OrgID.IsNull() && !config.OrgID.IsUnknown() && (candidate.OrgID == nil || *candidate.OrgID != config.OrgID.ValueString()) {
				continue
			}
			if role != nil {
				resp.Diagnostics.AddError("Ambiguous Role", "Multiple active roles have the name "+name+". Set org_id for a custom role or look up a system role by ID.")
				return
			}
			role = candidate
		}
		if role == nil {
			resp.Diagnostics.AddError("Role Not Found", "No active role found with name: "+name)
			return
		}
	}
	var state roleModel
	mapRoleToModel(role, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
