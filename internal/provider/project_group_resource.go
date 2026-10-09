package provider

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/braintrustdata/terraform-provider-braintrust/internal/client"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource                = &projectGroupResource{}
	_ resource.ResourceWithConfigure   = &projectGroupResource{}
	_ resource.ResourceWithImportState = &projectGroupResource{}
)

func NewProjectGroupResource() resource.Resource { return &projectGroupResource{} }

type projectGroupResource struct{ client *client.Client }

func (r *projectGroupResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_project_group"
}

func (r *projectGroupResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Braintrust project group and its member projects. A project may belong to multiple project groups.",
		Attributes: map[string]schema.Attribute{
			"id": computedStringAttribute("Project group ID."),
			"org_id": schema.StringAttribute{
				Required: true, Description: "Organization ID for this project group.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"name":        schema.StringAttribute{Required: true, Description: "Name, unique within the organization.", Validators: []validator.String{stringvalidator.LengthAtLeast(1)}},
			"description": optionalComputedStringAttribute("Description of the project group."),
			"created":     computedStringAttribute("Creation timestamp."),
			"deleted_at":  computedStringAttribute("Deletion timestamp, or null."),
			"user_id":     computedStringAttribute("Creating user ID."),
			"member_projects": schema.SetAttribute{
				Optional: true, Computed: true, ElementType: types.StringType,
				Description:   "Project IDs in this group. Terraform references to managed projects establish create and destroy ordering.",
				PlanModifiers: []planmodifier.Set{setplanmodifier.UseStateForUnknown()},
			},
		},
	}
}

func (r *projectGroupResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = clientFromProviderData(req.ProviderData, &resp.Diagnostics)
}

func (r *projectGroupResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan projectGroupModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() || !requireClient(r.client, &resp.Diagnostics) {
		return
	}
	add := stringSetFromModel(plan.MemberProjects)
	// POST returns an existing group for a duplicate name. Refuse to adopt and
	// mutate an object owned outside this Terraform state.
	existing, err := r.client.ListProjectGroups(ctx, plan.OrgID.ValueString(), plan.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error Checking Project Group Name", err.Error())
		return
	}
	for _, group := range existing {
		if group.OrgID == plan.OrgID.ValueString() && group.Name == plan.Name.ValueString() && group.DeletedAt == nil {
			resp.Diagnostics.AddError("Project Group Already Exists", "Import the existing project group instead of creating another with the same organization and name.")
			return
		}
	}
	created, err := r.client.CreateProjectGroup(ctx, client.CreateProjectGroupRequest{
		OrgID: plan.OrgID.ValueString(), Name: plan.Name.ValueString(), Description: tfStringToPtr(plan.Description),
	})
	if err != nil {
		resp.Diagnostics.AddError("Error Creating Project Group", err.Error())
		return
	}
	// POST cannot set membership. Persist the created ID before PATCH so a failed
	// membership update can be retried or destroyed without orphaning the group.
	mapProjectGroupToModel(created, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	group := created
	if len(add) > 0 {
		group, err = r.applyChanges(ctx, created.ID, client.UpdateProjectGroupRequest{AddMemberProjects: add})
		if err != nil {
			resp.Diagnostics.AddError("Error Adding Project Group Members", fmt.Sprintf("Project group %s was created: %v", created.ID, err))
			return
		}
	}
	group, err = r.client.GetProjectGroup(ctx, group.ID)
	if err != nil {
		resp.Diagnostics.AddError("Error Reading Project Group After Create", fmt.Sprintf("Project group %s was created: %v", created.ID, err))
		return
	}
	mapProjectGroupToModel(group, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *projectGroupResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state projectGroupModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() || !requireClient(r.client, &resp.Diagnostics) {
		return
	}
	group, err := r.client.GetProjectGroup(ctx, state.ID.ValueString())
	if err != nil {
		if client.IsNotFoundError(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error Reading Project Group", err.Error())
		return
	}
	mapProjectGroupToModel(group, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *projectGroupResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state projectGroupModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() || !requireClient(r.client, &resp.Diagnostics) {
		return
	}
	add, remove := diffStringSlices(stringSetFromModel(state.MemberProjects), stringSetFromModel(plan.MemberProjects))
	name := plan.Name.ValueString()
	update := client.UpdateProjectGroupRequest{Name: &name, AddMemberProjects: add, RemoveMemberProjects: remove}
	if plan.Description.IsNull() && !state.Description.IsNull() {
		null := json.RawMessage("null")
		update.Description = &null
	} else if !plan.Description.IsNull() && !plan.Description.IsUnknown() {
		value, _ := json.Marshal(plan.Description.ValueString())
		raw := json.RawMessage(value)
		update.Description = &raw
	}
	group, err := r.applyChanges(ctx, state.ID.ValueString(), update)
	if err != nil {
		resp.Diagnostics.AddError("Error Updating Project Group", err.Error())
		return
	}
	mapProjectGroupToModel(group, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

// The API accepts at most 1,000 distinct membership changes per PATCH.
// Apply metadata on the first batch; a later failure is recovered by Read,
// which observes the successfully applied batches before the next plan.
func (r *projectGroupResource) applyChanges(ctx context.Context, id string, change client.UpdateProjectGroupRequest) (*client.ProjectGroup, error) {
	const batchSize = 1000
	var last *client.ProjectGroup
	for first := true; first || len(change.AddMemberProjects) > 0 || len(change.RemoveMemberProjects) > 0; first = false {
		batch := client.UpdateProjectGroupRequest{}
		if first {
			batch.Name, batch.Description = change.Name, change.Description
		}
		remaining := batchSize
		if len(change.RemoveMemberProjects) > 0 {
			n := min(remaining, len(change.RemoveMemberProjects))
			batch.RemoveMemberProjects = change.RemoveMemberProjects[:n]
			change.RemoveMemberProjects = change.RemoveMemberProjects[n:]
			remaining -= n
		}
		if remaining > 0 && len(change.AddMemberProjects) > 0 {
			n := min(remaining, len(change.AddMemberProjects))
			batch.AddMemberProjects = change.AddMemberProjects[:n]
			change.AddMemberProjects = change.AddMemberProjects[n:]
		}
		var err error
		last, err = r.client.UpdateProjectGroup(ctx, id, batch)
		if err != nil {
			return nil, err
		}
	}
	return last, nil
}

func (r *projectGroupResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state projectGroupModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() || !requireClient(r.client, &resp.Diagnostics) {
		return
	}
	if err := r.client.DeleteProjectGroup(ctx, state.ID.ValueString()); err != nil && !client.IsNotFoundError(err) {
		resp.Diagnostics.AddError("Error Deleting Project Group", err.Error())
	}
}

func (r *projectGroupResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
