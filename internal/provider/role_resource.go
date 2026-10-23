package provider

import (
	"context"

	"github.com/braintrustdata/braintrust-sdk-go/openapi"
	"github.com/braintrustdata/terraform-provider-braintrust/internal/client"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/oapi-codegen/nullable"
)

var (
	_ resource.Resource                = &roleResource{}
	_ resource.ResourceWithConfigure   = &roleResource{}
	_ resource.ResourceWithImportState = &roleResource{}
)

// NewRoleResource returns a new role resource.
func NewRoleResource() resource.Resource {
	return &roleResource{}
}

type roleResource struct {
	client *client.Client
}

func (r *roleResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_role"
}

func (r *roleResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Version:             0,
		Description:         "Manages a custom Braintrust role.",
		MarkdownDescription: "Manages a custom Braintrust role. Roles are collections of permissions that can be granted via ACLs. Use the `braintrust_role` data source to reference built-in system roles.",
		Attributes: map[string]schema.Attribute{
			"id":         computedStringAttribute("Unique identifier for the role (UUID)."),
			"org_id":     computedStringAttribute("Organization the role belongs to."),
			"created":    computedStringAttribute("Date of role creation."),
			"deleted_at": computedStringAttribute("Date of role deletion, or null if the role is still active."),
			"user_id":    computedStringAttribute("Identifies the user who created the role."),
			"name": schema.StringAttribute{
				Required:    true,
				Description: "Name of the role.",
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"description": optionalComputedStringAttribute("Textual description of the role."),
			"member_permissions": schema.SetNestedAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Permissions that belong to this role. Each entry is a (permission, restrict_object_type) tuple.",
				PlanModifiers: []planmodifier.Set{
					setplanmodifier.UseStateForUnknown(),
				},
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"permission": schema.StringAttribute{
							Required:    true,
							Description: "The permission type (create, read, update, delete, create_acls, read_acls, update_acls, delete_acls).",
							Validators: []validator.String{
								stringvalidator.OneOf(
									"create", "read", "update", "delete",
									"create_acls", "read_acls", "update_acls", "delete_acls",
								),
							},
						},
						"restrict_object_type": schema.StringAttribute{
							Optional:    true,
							Description: "Optionally restricts the permission to a specific object type.",
							Validators: []validator.String{
								stringvalidator.OneOf(aclObjectTypes...),
							},
						},
					},
				},
			},
			"member_roles": schema.SetAttribute{
				Optional:    true,
				Computed:    true,
				Description: "IDs of roles this role inherits from. An inheriting role has all the permissions of its member roles.",
				ElementType: types.StringType,
				PlanModifiers: []planmodifier.Set{
					setplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

func (r *roleResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = clientFromProviderData(req.ProviderData, &resp.Diagnostics)
}

func (r *roleResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan roleModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !requireClient(r.client, &resp.Diagnostics) {
		return
	}

	memberPermissions, diags := memberPermissionsFromModel(ctx, plan.MemberPermissions)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	memberRoles, err := parseUUIDs(stringSetFromModel(plan.MemberRoles), "member_roles")
	if err != nil {
		resp.Diagnostics.AddAttributeError(path.Root("member_roles"), "Invalid Member Roles", err.Error())
		return
	}
	createReq := openapi.CreateRole{
		Name:              plan.Name.ValueString(),
		Description:       tfStringToNullable(plan.Description),
		MemberPermissions: nullableSlice(memberPermissions),
		MemberRoles:       nullableSlice(memberRoles),
	}

	tflog.Debug(ctx, "Creating role", map[string]any{"name": createReq.Name})

	createResp, err := client.Write(r.client.API.PostRoleWithResponse(ctx, createReq))
	var created *openapi.Role
	if err == nil {
		created, err = client.Created(createResp.HTTPResponse, createResp.JSON200, "role")
	}
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Creating Braintrust Role",
			"Could not create role, unexpected error: "+err.Error(),
		)
		return
	}

	// Re-read to get the full state.
	role, err := getRole(ctx, r.client, created.Id.String())
	if err != nil {
		mapRoleToModel(created, &plan)
		resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
		resp.Diagnostics.AddError(
			"Error Reading Braintrust Role After Create",
			"Role was created (ID: "+created.Id.String()+") but could not be read: "+err.Error(),
		)
		return
	}

	mapRoleToModel(role, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *roleResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state roleModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !requireClient(r.client, &resp.Diagnostics) {
		return
	}

	role, err := getRole(ctx, r.client, state.ID.ValueString())
	if err != nil {
		if client.IsNotFoundError(err) {
			tflog.Warn(ctx, "Role not found, removing from state", map[string]any{
				"id": state.ID.ValueString(),
			})
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Error Reading Braintrust Role",
			"Could not read role ID "+state.ID.ValueString()+": "+err.Error(),
		)
		return
	}

	mapRoleToModel(role, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *roleResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan roleModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var state roleModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !requireClient(r.client, &resp.Diagnostics) {
		return
	}

	// Compute add/remove diffs for member_permissions and member_roles.
	currentPerms, diags := memberPermissionsFromModel(ctx, state.MemberPermissions)
	resp.Diagnostics.Append(diags...)
	desiredPerms, diags := memberPermissionsFromModel(ctx, plan.MemberPermissions)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	addPerms, removePerms := diffMemberPermissions(currentPerms, desiredPerms)

	currentRoles := stringSetFromModel(state.MemberRoles)
	desiredRoles := stringSetFromModel(plan.MemberRoles)
	addRoles, removeRoles, err := diffUUIDs(currentRoles, desiredRoles, "member_roles")
	if err != nil {
		resp.Diagnostics.AddAttributeError(path.Root("member_roles"), "Invalid Member Roles", err.Error())
		return
	}

	updateReq := openapi.PatchRole{
		Name:                    nullable.NewNullableWithValue(plan.Name.ValueString()),
		Description:             patchString(plan.Description, state.Description),
		AddMemberPermissions:    nullableSlice(addPerms),
		RemoveMemberPermissions: nullableSlice(removePerms),
		AddMemberRoles:          nullableSlice(addRoles),
		RemoveMemberRoles:       nullableSlice(removeRoles),
	}

	tflog.Debug(ctx, "Updating role", map[string]any{"id": state.ID.ValueString()})

	updateResp, err := client.Write(r.client.API.PatchRoleIdWithResponse(ctx, state.ID.ValueString(), updateReq))
	var role *openapi.Role
	if err == nil {
		role, err = client.Written(updateResp.JSON200)
	}
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Updating Braintrust Role",
			"Could not update role ID "+state.ID.ValueString()+": "+err.Error(),
		)
		return
	}

	mapRoleToModel(role, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *roleResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state roleModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !requireClient(r.client, &resp.Diagnostics) {
		return
	}

	_, err := client.Write(r.client.API.DeleteRoleIdWithResponse(ctx, state.ID.ValueString()))
	if err != nil {
		if client.IsNotFoundError(err) {
			return
		}
		resp.Diagnostics.AddError(
			"Error Deleting Braintrust Role",
			"Could not delete role ID "+state.ID.ValueString()+": "+err.Error(),
		)
	}
}

func (r *roleResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
