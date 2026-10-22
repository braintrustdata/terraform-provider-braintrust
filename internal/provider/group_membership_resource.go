package provider

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/braintrustdata/braintrust-sdk-go/openapi"
	"github.com/braintrustdata/terraform-provider-braintrust/internal/client"
	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource                = &groupMembershipResource{}
	_ resource.ResourceWithConfigure   = &groupMembershipResource{}
	_ resource.ResourceWithImportState = &groupMembershipResource{}
)

func NewGroupMembershipResource() resource.Resource { return &groupMembershipResource{} }

type groupMembershipResource struct{ client *client.Client }
type groupMembershipModel struct {
	ID      types.String `tfsdk:"id"`
	GroupID types.String `tfsdk:"group_id"`
	UserID  types.String `tfsdk:"user_id"`
}

func (r *groupMembershipResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_group_membership"
}

func (r *groupMembershipResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	replace := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	resp.Schema = schema.Schema{
		Description: "Manages one user's membership in a Braintrust permission group without managing the group's other members. Use for built-in groups or leave member_users unset on a managed group. Import with group_id/user_id.",
		Attributes: map[string]schema.Attribute{
			"id":       computedStringAttribute("Composite ID in group_id/user_id form."),
			"group_id": schema.StringAttribute{Required: true, PlanModifiers: replace, Description: "Permission group ID."},
			"user_id":  schema.StringAttribute{Required: true, PlanModifiers: replace, Description: "Member user ID."},
		},
	}
}

func (r *groupMembershipResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = clientFromProviderData(req.ProviderData, &resp.Diagnostics)
}

func groupMembershipID(groupID, userID string) string { return groupID + "/" + userID }

func (r *groupMembershipResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan groupMembershipModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() || !requireClient(r.client, &resp.Diagnostics) {
		return
	}
	groupID, userID := plan.GroupID.ValueString(), plan.UserID.ValueString()
	userUUID, err := parseUUID(plan.UserID, "user_id")
	if err != nil {
		resp.Diagnostics.AddAttributeError(path.Root("user_id"), "Invalid User ID", err.Error())
		return
	}
	group, err := getGroup(ctx, r.client, groupID)
	if err != nil {
		resp.Diagnostics.AddError("Error Reading Group", err.Error())
		return
	}
	if slices.Contains(groupMemberUsers(group), userID) {
		resp.Diagnostics.AddError("Group Membership Already Exists", "Import the existing membership instead of creating it again.")
		return
	}
	plan.ID = types.StringValue(groupMembershipID(groupID, userID))
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	group, err = patchGroup(ctx, r.client, groupID, openapi.PatchGroup{AddMemberUsers: nullableSlice([]uuid.UUID{userUUID})})
	if err != nil {
		resp.Diagnostics.AddError("Error Adding Group Membership", err.Error())
		return
	}
	if !slices.Contains(groupMemberUsers(group), userID) {
		resp.Diagnostics.AddError("Group Membership Missing After Add", fmt.Sprintf("User %s was not returned as a member of group %s", userID, groupID))
	}
}

func (r *groupMembershipResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state groupMembershipModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() || !requireClient(r.client, &resp.Diagnostics) {
		return
	}
	group, err := getGroup(ctx, r.client, state.GroupID.ValueString())
	if err != nil {
		if client.IsNotFoundError(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error Reading Group Membership", err.Error())
		return
	}
	if !slices.Contains(groupMemberUsers(group), state.UserID.ValueString()) {
		resp.State.RemoveResource(ctx)
		return
	}
	state.ID = types.StringValue(groupMembershipID(state.GroupID.ValueString(), state.UserID.ValueString()))
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *groupMembershipResource) Update(_ context.Context, _ resource.UpdateRequest, resp *resource.UpdateResponse) {
	resp.Diagnostics.AddError("Unexpected Membership Update", "Membership IDs are immutable and must be replaced.")
}

func (r *groupMembershipResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state groupMembershipModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() || !requireClient(r.client, &resp.Diagnostics) {
		return
	}
	groupID, userID := state.GroupID.ValueString(), state.UserID.ValueString()
	group, err := getGroup(ctx, r.client, groupID)
	if err != nil {
		if client.IsNotFoundError(err) {
			return
		}
		resp.Diagnostics.AddError("Error Reading Group Membership", err.Error())
		return
	}
	if !slices.Contains(groupMemberUsers(group), userID) {
		return
	}
	userUUID, err := parseUUID(state.UserID, "user_id")
	if err != nil {
		resp.Diagnostics.AddError("Error Removing Group Membership", err.Error())
		return
	}
	_, err = patchGroup(ctx, r.client, groupID, openapi.PatchGroup{RemoveMemberUsers: nullableSlice([]uuid.UUID{userUUID})})
	if err != nil {
		resp.Diagnostics.AddError("Error Removing Group Membership", err.Error())
	}
}

func (r *groupMembershipResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.Split(req.ID, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		resp.Diagnostics.AddError("Invalid Membership Import ID", "Use group_id/user_id.")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("group_id"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("user_id"), parts[1])...)
}
