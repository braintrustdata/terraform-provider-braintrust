package provider

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/braintrustdata/terraform-provider-braintrust/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource                = &projectGroupMembershipResource{}
	_ resource.ResourceWithConfigure   = &projectGroupMembershipResource{}
	_ resource.ResourceWithImportState = &projectGroupMembershipResource{}
)

func NewProjectGroupMembershipResource() resource.Resource { return &projectGroupMembershipResource{} }

type projectGroupMembershipResource struct{ client *client.Client }
type projectGroupMembershipModel struct {
	ID             types.String `tfsdk:"id"`
	ProjectGroupID types.String `tfsdk:"project_group_id"`
	ProjectID      types.String `tfsdk:"project_id"`
}

func (r *projectGroupMembershipResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_project_group_membership"
}

func (r *projectGroupMembershipResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	replace := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	resp.Schema = schema.Schema{
		Description: "Manages one project membership in a Braintrust project group. Use this resource when Terraform also manages the project, so membership is removed before project deletion. Do not also set member_projects on the group resource.",
		MarkdownDescription: "Manages one project membership in a Braintrust project group. Reference both the group and project so Terraform removes membership before destroying either object. " +
			"Do not also set `member_projects` on the group resource. The API requires `create_acls` on the project to add it and `delete_acls` to remove it. " +
			"Import with `project_group_id/project_id`.",
		Attributes: map[string]schema.Attribute{
			"id":               computedStringAttribute("Composite ID in project_group_id/project_id form."),
			"project_group_id": schema.StringAttribute{Required: true, PlanModifiers: replace, Description: "Project group ID."},
			"project_id":       schema.StringAttribute{Required: true, PlanModifiers: replace, Description: "Member project ID."},
		},
	}
}

func (r *projectGroupMembershipResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = clientFromProviderData(req.ProviderData, &resp.Diagnostics)
}

func membershipID(groupID, projectID string) string { return groupID + "/" + projectID }

func (r *projectGroupMembershipResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan projectGroupMembershipModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() || !requireClient(r.client, &resp.Diagnostics) {
		return
	}
	groupID, projectID := plan.ProjectGroupID.ValueString(), plan.ProjectID.ValueString()
	group, err := r.client.GetProjectGroup(ctx, groupID)
	if err != nil {
		resp.Diagnostics.AddError("Error Reading Project Group", err.Error())
		return
	}
	if slices.Contains(group.MemberProjects, projectID) {
		resp.Diagnostics.AddError("Project Group Membership Already Exists", "Import the existing membership instead of creating it again.")
		return
	}
	// Keep the relationship address in state if the PATCH commits but its
	// response fails, so the next refresh can determine what happened.
	plan.ID = types.StringValue(membershipID(groupID, projectID))
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	group, err = r.client.UpdateProjectGroup(ctx, groupID, client.UpdateProjectGroupRequest{AddMemberProjects: []string{projectID}})
	if err != nil {
		resp.Diagnostics.AddError("Error Adding Project Group Membership", err.Error())
		return
	}
	if !slices.Contains(group.MemberProjects, projectID) {
		resp.Diagnostics.AddError("Project Group Membership Missing After Add", fmt.Sprintf("Project %s was not returned as a member of group %s", projectID, groupID))
	}
}

func (r *projectGroupMembershipResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state projectGroupMembershipModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() || !requireClient(r.client, &resp.Diagnostics) {
		return
	}
	group, err := r.client.GetProjectGroup(ctx, state.ProjectGroupID.ValueString())
	if err != nil {
		if client.IsNotFoundError(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error Reading Project Group Membership", err.Error())
		return
	}
	if !slices.Contains(group.MemberProjects, state.ProjectID.ValueString()) {
		resp.State.RemoveResource(ctx)
		return
	}
	state.ID = types.StringValue(membershipID(state.ProjectGroupID.ValueString(), state.ProjectID.ValueString()))
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *projectGroupMembershipResource) Update(_ context.Context, _ resource.UpdateRequest, resp *resource.UpdateResponse) {
	resp.Diagnostics.AddError("Unexpected Membership Update", "Membership IDs are immutable and must be replaced.")
}

func (r *projectGroupMembershipResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state projectGroupMembershipModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() || !requireClient(r.client, &resp.Diagnostics) {
		return
	}
	groupID, projectID := state.ProjectGroupID.ValueString(), state.ProjectID.ValueString()
	group, err := r.client.GetProjectGroup(ctx, groupID)
	if err != nil {
		if client.IsNotFoundError(err) {
			return
		}
		resp.Diagnostics.AddError("Error Reading Project Group Membership", err.Error())
		return
	}
	if !slices.Contains(group.MemberProjects, projectID) {
		return
	}
	_, err = r.client.UpdateProjectGroup(ctx, groupID, client.UpdateProjectGroupRequest{RemoveMemberProjects: []string{projectID}})
	if err != nil {
		resp.Diagnostics.AddError("Error Removing Project Group Membership", err.Error())
	}
}

func (r *projectGroupMembershipResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.Split(req.ID, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		resp.Diagnostics.AddError("Invalid Membership Import ID", "Use project_group_id/project_id.")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("project_group_id"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("project_id"), parts[1])...)
}
