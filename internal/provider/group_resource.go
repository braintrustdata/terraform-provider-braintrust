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
	_ resource.Resource                = &groupResource{}
	_ resource.ResourceWithConfigure   = &groupResource{}
	_ resource.ResourceWithImportState = &groupResource{}
)

// NewGroupResource returns a new group resource.
func NewGroupResource() resource.Resource {
	return &groupResource{}
}

type groupResource struct {
	client *client.Client
}

func (r *groupResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_group"
}

func (r *groupResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Version:             0,
		Description:         "Manages a Braintrust group.",
		MarkdownDescription: "Manages a Braintrust group. Groups are collections of users that can be assigned permissions via ACLs. Groups can also inherit from other groups.",
		Attributes: map[string]schema.Attribute{
			"id":         computedStringAttribute("Unique identifier for the group (UUID)."),
			"org_id":     computedStringAttribute("Organization the group belongs to."),
			"created":    computedStringAttribute("Date of group creation."),
			"deleted_at": computedStringAttribute("Date of group deletion, or null if the group is still active."),
			"user_id":    computedStringAttribute("Identifies the user who created the group."),
			"name": schema.StringAttribute{
				Required:    true,
				Description: "Name of the group. Must be unique within the organization.",
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"description": optionalComputedStringAttribute("Textual description of the group."),
			"member_users": schema.SetAttribute{
				Optional:    true,
				Computed:    true,
				Description: "IDs of users in this group.",
				ElementType: types.StringType,
				PlanModifiers: []planmodifier.Set{
					setplanmodifier.UseStateForUnknown(),
				},
			},
			"member_groups": schema.SetAttribute{
				Optional:    true,
				Computed:    true,
				Description: "IDs of groups this group inherits from. An inheriting group has all the users of its member groups.",
				ElementType: types.StringType,
				PlanModifiers: []planmodifier.Set{
					setplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

func (r *groupResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = clientFromProviderData(req.ProviderData, &resp.Diagnostics)
}

func (r *groupResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan groupModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !requireClient(r.client, &resp.Diagnostics) {
		return
	}

	memberUsers, err := parseUUIDs(stringSetFromModel(plan.MemberUsers), "member_users")
	if err != nil {
		resp.Diagnostics.AddAttributeError(path.Root("member_users"), "Invalid Member Users", err.Error())
		return
	}
	memberGroups, err := parseUUIDs(stringSetFromModel(plan.MemberGroups), "member_groups")
	if err != nil {
		resp.Diagnostics.AddAttributeError(path.Root("member_groups"), "Invalid Member Groups", err.Error())
		return
	}
	createReq := openapi.CreateGroup{
		Name:         plan.Name.ValueString(),
		Description:  tfStringToNullable(plan.Description),
		MemberUsers:  nullableSlice(memberUsers),
		MemberGroups: nullableSlice(memberGroups),
	}

	tflog.Debug(ctx, "Creating group", map[string]any{"name": createReq.Name})

	// POST returns an existing same-name group without x-bt-found-existing.
	existing, err := listGroups(ctx, r.client, createReq.Name, "")
	if err != nil {
		resp.Diagnostics.AddError("Error Checking Existing Groups", err.Error())
		return
	}
	createResp, err := client.Write(r.client.API.PostGroupWithResponse(ctx, createReq))
	var created *openapi.Group
	if err == nil {
		created, err = client.Created(createResp.HTTPResponse, createResp.JSON200, "group")
	}
	if err == nil {
		err = rejectPreexisting(created, existing, func(g openapi.Group) string { return g.Id.String() }, "group")
	}
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Creating Braintrust Group",
			"Could not create group, unexpected error: "+err.Error(),
		)
		return
	}

	// Re-read to get the full state.
	group, err := getGroup(ctx, r.client, created.Id.String())
	if err != nil {
		mapGroupToModel(created, &plan)
		resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
		resp.Diagnostics.AddError(
			"Error Reading Braintrust Group After Create",
			"Group was created (ID: "+created.Id.String()+") but could not be read: "+err.Error(),
		)
		return
	}

	mapGroupToModel(group, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *groupResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state groupModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !requireClient(r.client, &resp.Diagnostics) {
		return
	}

	group, err := getGroup(ctx, r.client, state.ID.ValueString())
	if err != nil {
		if client.IsNotFoundError(err) {
			tflog.Warn(ctx, "Group not found, removing from state", map[string]any{
				"id": state.ID.ValueString(),
			})
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Error Reading Braintrust Group",
			"Could not read group ID "+state.ID.ValueString()+": "+err.Error(),
		)
		return
	}

	mapGroupToModel(group, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *groupResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan groupModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var state groupModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !requireClient(r.client, &resp.Diagnostics) {
		return
	}

	// Compute add/remove diffs for members.
	addUsers, removeUsers, err := diffUUIDs(stringSetFromModel(state.MemberUsers), stringSetFromModel(plan.MemberUsers), "member_users")
	if err != nil {
		resp.Diagnostics.AddAttributeError(path.Root("member_users"), "Invalid Member Users", err.Error())
		return
	}
	addGroups, removeGroups, err := diffUUIDs(stringSetFromModel(state.MemberGroups), stringSetFromModel(plan.MemberGroups), "member_groups")
	if err != nil {
		resp.Diagnostics.AddAttributeError(path.Root("member_groups"), "Invalid Member Groups", err.Error())
		return
	}

	updateReq := openapi.PatchGroup{
		Name:               nullable.NewNullableWithValue(plan.Name.ValueString()),
		Description:        patchString(plan.Description, state.Description),
		AddMemberUsers:     nullableSlice(addUsers),
		RemoveMemberUsers:  nullableSlice(removeUsers),
		AddMemberGroups:    nullableSlice(addGroups),
		RemoveMemberGroups: nullableSlice(removeGroups),
	}

	tflog.Debug(ctx, "Updating group", map[string]any{"id": state.ID.ValueString()})

	group, err := patchGroup(ctx, r.client, state.ID.ValueString(), updateReq)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Updating Braintrust Group",
			"Could not update group ID "+state.ID.ValueString()+": "+err.Error(),
		)
		return
	}

	mapGroupToModel(group, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *groupResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state groupModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !requireClient(r.client, &resp.Diagnostics) {
		return
	}

	_, err := client.Write(r.client.API.DeleteGroupIdWithResponse(ctx, state.ID.ValueString()))
	if err != nil {
		if client.IsNotFoundError(err) {
			return
		}
		resp.Diagnostics.AddError(
			"Error Deleting Braintrust Group",
			"Could not delete group ID "+state.ID.ValueString()+": "+err.Error(),
		)
	}
}

func (r *groupResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
