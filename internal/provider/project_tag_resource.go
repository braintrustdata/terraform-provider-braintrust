package provider

import (
	"context"

	"github.com/braintrustdata/terraform-provider-braintrust/internal/client"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

var (
	_ resource.Resource                = &projectTagResource{}
	_ resource.ResourceWithConfigure   = &projectTagResource{}
	_ resource.ResourceWithImportState = &projectTagResource{}
)

// NewProjectTagResource returns a new project tag resource.
func NewProjectTagResource() resource.Resource {
	return &projectTagResource{}
}

type projectTagResource struct {
	client *client.Client
}

func (r *projectTagResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_project_tag"
}

func (r *projectTagResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Version:             0,
		Description:         "Manages a Braintrust project tag.",
		MarkdownDescription: "Manages a Braintrust project tag. Tags are user-configured labels for tracking and filtering experiments, logs, and other data within a project.",
		Attributes: map[string]schema.Attribute{
			"id":      computedStringAttribute("Unique identifier for the project tag (UUID)."),
			"user_id": computedStringAttribute("Identifies the user who created the project tag."),
			"created": computedStringAttribute("Date of project tag creation."),
			"project_id": schema.StringAttribute{
				Required:    true,
				Description: "Unique identifier for the project that the tag belongs to.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"name": schema.StringAttribute{
				Required:    true,
				Description: "Name of the project tag.",
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"description": optionalComputedStringAttribute("Textual description of the project tag."),
			"color": schema.StringAttribute{
				Optional:    true,
				Description: "Color of the tag for the UI.",
			},
		},
	}
}

func (r *projectTagResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = clientFromProviderData(req.ProviderData, &resp.Diagnostics)
}

func (r *projectTagResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan projectTagModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !requireClient(r.client, &resp.Diagnostics) {
		return
	}

	createReq := client.CreateProjectTagRequest{
		ProjectID:   plan.ProjectID.ValueString(),
		Name:        plan.Name.ValueString(),
		Description: tfStringToPtr(plan.Description),
		Color:       tfStringToPtr(plan.Color),
	}

	tflog.Debug(ctx, "Creating project tag", map[string]any{
		"project_id": createReq.ProjectID,
		"name":       createReq.Name,
	})

	created, err := r.client.CreateProjectTag(ctx, createReq)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Creating Braintrust Project Tag",
			"Could not create project tag, unexpected error: "+err.Error(),
		)
		return
	}

	// Re-read to get the full state.
	tag, err := r.client.GetProjectTag(ctx, created.ID)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Reading Braintrust Project Tag After Create",
			"Tag was created (ID: "+created.ID+") but could not be read: "+err.Error(),
		)
		return
	}

	mapProjectTagToModel(tag, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *projectTagResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state projectTagModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !requireClient(r.client, &resp.Diagnostics) {
		return
	}

	tag, err := r.client.GetProjectTag(ctx, state.ID.ValueString())
	if err != nil {
		if client.IsNotFoundError(err) {
			tflog.Warn(ctx, "Project tag not found, removing from state", map[string]any{
				"id": state.ID.ValueString(),
			})
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Error Reading Braintrust Project Tag",
			"Could not read project tag ID "+state.ID.ValueString()+": "+err.Error(),
		)
		return
	}

	mapProjectTagToModel(tag, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *projectTagResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan projectTagModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var state projectTagModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !requireClient(r.client, &resp.Diagnostics) {
		return
	}

	name := plan.Name.ValueString()
	updateReq := client.UpdateProjectTagRequest{
		Name: &name,
	}

	if !plan.Description.IsNull() && !plan.Description.IsUnknown() {
		desc := plan.Description.ValueString()
		updateReq.Description = &desc
	} else if plan.Description.IsNull() && !state.Description.IsNull() {
		empty := ""
		updateReq.Description = &empty
	}

	if !plan.Color.IsNull() && !plan.Color.IsUnknown() {
		color := plan.Color.ValueString()
		updateReq.Color = &color
	} else if plan.Color.IsNull() && !state.Color.IsNull() {
		empty := ""
		updateReq.Color = &empty
	}

	tag, err := r.client.UpdateProjectTag(ctx, state.ID.ValueString(), updateReq)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Updating Braintrust Project Tag",
			"Could not update project tag ID "+state.ID.ValueString()+": "+err.Error(),
		)
		return
	}

	mapProjectTagToModel(tag, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *projectTagResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state projectTagModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !requireClient(r.client, &resp.Diagnostics) {
		return
	}

	err := r.client.DeleteProjectTag(ctx, state.ID.ValueString())
	if err != nil {
		if client.IsNotFoundError(err) {
			return
		}
		resp.Diagnostics.AddError(
			"Error Deleting Braintrust Project Tag",
			"Could not delete project tag ID "+state.ID.ValueString()+": "+err.Error(),
		)
	}
}

func (r *projectTagResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
