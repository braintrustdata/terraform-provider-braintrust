package provider

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/braintrustdata/braintrust-sdk-go/openapi"
	"github.com/braintrustdata/terraform-provider-braintrust/internal/client"
	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/oapi-codegen/nullable"
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
		MarkdownDescription: "Manages a Braintrust project group. A project may belong to multiple project groups.\n\n" +
			"For projects also managed by Terraform, leave `member_projects` unset and manage each relationship with `braintrust_project_group_membership`. " +
			"This ensures Terraform removes membership before destroying the project or group. Inline `member_projects` requires removing membership in a separate apply before destroying a project; do not use both ownership styles for one group.\n\n" +
			"Adding projects requires `create_acls` on them, and removing projects or deleting a populated group requires `delete_acls`.",
		Attributes: map[string]schema.Attribute{
			"id": computedStringAttribute("Unique identifier for the project group (UUID)."),
			"org_id": schema.StringAttribute{
				Required: true, Description: "Organization ID for this project group.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"name":        schema.StringAttribute{Required: true, Description: "Name of the project group. Must be unique within the organization.", Validators: []validator.String{stringvalidator.LengthAtLeast(1)}},
			"description": optionalComputedStringAttribute("Textual description of the project group."),
			"created":     computedStringAttribute("Date of project group creation."),
			"deleted_at":  computedStringAttribute("Date of project group deletion, or null if the project group is still active."),
			"user_id":     computedStringAttribute("Identifies the user who created the project group."),
			"member_projects": schema.SetAttribute{
				Optional: true, Computed: true, ElementType: types.StringType,
				Description:   "Project IDs managed inline. Use separate membership resources for safe simultaneous project deletion.",
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
	add, err := parseUUIDs(stringSetFromModel(plan.MemberProjects), "member_projects")
	if err != nil {
		resp.Diagnostics.AddAttributeError(path.Root("member_projects"), "Invalid Member Projects", err.Error())
		return
	}
	// POST returns an existing group for a duplicate name. Refuse to adopt and
	// mutate an object owned outside this Terraform state.
	existing, err := listProjectGroups(ctx, r.client, plan.OrgID.ValueString(), plan.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error Checking Project Group Name", err.Error())
		return
	}
	for _, group := range existing {
		if group.OrgId.String() == plan.OrgID.ValueString() && group.Name == plan.Name.ValueString() && !nullableIsSet(group.DeletedAt) {
			resp.Diagnostics.AddError("Project Group Already Exists", "Import the existing project group instead of creating another with the same organization and name.")
			return
		}
	}
	// The public spec only documents org_name, but the API accepts org_id,
	// which is what this resource is keyed on.
	orgID, err := json.Marshal(plan.OrgID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error Creating Project Group", err.Error())
		return
	}
	body, err := client.JSONBody(openapi.CreateProjectGroup{
		Name: plan.Name.ValueString(), Description: tfStringToNullable(plan.Description),
	}, map[string]json.RawMessage{"org_id": orgID})
	if err != nil {
		resp.Diagnostics.AddError("Error Creating Project Group", err.Error())
		return
	}
	createResp, err := client.Write(r.client.API.PostProjectGroupWithBodyWithResponse(ctx, client.JSONContentType, body))
	var created *openapi.ProjectGroup
	if err == nil {
		created, err = client.Created(createResp.HTTPResponse, createResp.JSON200, "project group")
	}
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
		group, err = r.applyChanges(ctx, created.Id.String(), projectGroupChange{add: add})
		if err != nil {
			resp.Diagnostics.AddError("Error Adding Project Group Members", fmt.Sprintf("Project group %s was created: %v", created.Id, err))
			return
		}
	}
	group, err = getProjectGroup(ctx, r.client, group.Id.String())
	if err != nil {
		resp.Diagnostics.AddError("Error Reading Project Group After Create", fmt.Sprintf("Project group %s was created: %v", created.Id, err))
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
	group, err := getProjectGroup(ctx, r.client, state.ID.ValueString())
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
	add, remove, err := diffUUIDs(stringSetFromModel(state.MemberProjects), stringSetFromModel(plan.MemberProjects), "member_projects")
	if err != nil {
		resp.Diagnostics.AddAttributeError(path.Root("member_projects"), "Invalid Member Projects", err.Error())
		return
	}
	update := projectGroupChange{name: nullable.NewNullableWithValue(plan.Name.ValueString()), add: add, remove: remove}
	if plan.Description.IsNull() && !state.Description.IsNull() {
		update.description = nullable.NewNullNullable[string]()
	} else if !plan.Description.IsNull() && !plan.Description.IsUnknown() {
		update.description = nullable.NewNullableWithValue(plan.Description.ValueString())
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
type projectGroupChange struct {
	name, description nullable.Nullable[string]
	add, remove       []uuid.UUID
}

func (r *projectGroupResource) applyChanges(ctx context.Context, id string, change projectGroupChange) (*openapi.ProjectGroup, error) {
	const batchSize = 1000
	var last *openapi.ProjectGroup
	for first := true; first || len(change.add) > 0 || len(change.remove) > 0; first = false {
		batch := openapi.PatchProjectGroup{}
		if first {
			batch.Name, batch.Description = change.name, change.description
		}
		remaining := batchSize
		if len(change.remove) > 0 {
			n := min(remaining, len(change.remove))
			batch.RemoveMemberProjects = nullableSlice(change.remove[:n])
			change.remove = change.remove[n:]
			remaining -= n
		}
		if remaining > 0 && len(change.add) > 0 {
			n := min(remaining, len(change.add))
			batch.AddMemberProjects = nullableSlice(change.add[:n])
			change.add = change.add[n:]
		}
		var err error
		last, err = patchProjectGroup(ctx, r.client, id, batch)
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
	if _, err := client.Write(r.client.API.DeleteProjectGroupIdWithResponse(ctx, state.ID.ValueString())); err != nil && !client.IsNotFoundError(err) {
		resp.Diagnostics.AddError("Error Deleting Project Group", err.Error())
	}
}

func (r *projectGroupResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
