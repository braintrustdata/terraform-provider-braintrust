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

// Ensure the implementation satisfies the expected interfaces.
var (
	_ resource.Resource                = &projectResource{}
	_ resource.ResourceWithConfigure   = &projectResource{}
	_ resource.ResourceWithImportState = &projectResource{}
)

// NewProjectResource returns a new project resource.
func NewProjectResource() resource.Resource {
	return &projectResource{}
}

// projectResource is the resource implementation.
type projectResource struct {
	client *client.Client
}

// Metadata returns the resource type name.
func (r *projectResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_project"
}

// Schema defines the schema for the resource.
func (r *projectResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description:         "Manages a Braintrust project.",
		MarkdownDescription: "Manages a Braintrust [project](https://www.braintrustdata.com/docs/guides/projects). Projects are the top-level organizational unit in Braintrust, containing experiments, datasets, prompts, and more.",
		Attributes: map[string]schema.Attribute{
			"id":         computedStringAttribute("Unique identifier for the project (UUID)."),
			"org_id":     computedStringAttribute("Unique identifier for the organization that the project belongs to."),
			"created":    computedStringAttribute("Date of project creation."),
			"deleted_at": computedStringAttribute("Date of project deletion, or null if the project is still active."),
			"user_id":    computedStringAttribute("Identifies the user who created the project."),
			"name": schema.StringAttribute{
				Required:    true,
				Description: "Name of the project.",
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"description": optionalComputedStringAttribute("Textual description of the project."),
			"comparison_key": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				Description:         "The key used to join two experiments (defaults to 'input').",
				MarkdownDescription: "The key used to join two experiments (defaults to `input`).",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"baseline_experiment_id": optionalComputedStringAttribute("The ID of the experiment to use as the default baseline for comparisons."),
		},
	}
}

// Configure adds the provider configured client to the resource.
func (r *projectResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = clientFromProviderData(req.ProviderData, &resp.Diagnostics)
}

// Create creates the resource and sets the initial Terraform state.
func (r *projectResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan projectModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !requireClient(r.client, &resp.Diagnostics) {
		return
	}

	// Build the create request.
	createReq := client.CreateProjectRequest{
		Name:        plan.Name.ValueString(),
		Description: tfStringToPtr(plan.Description),
	}

	tflog.Debug(ctx, "Creating project", map[string]any{
		"name": createReq.Name,
	})

	created, err := r.client.CreateProject(ctx, createReq)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Creating Braintrust Project",
			"Could not create project, unexpected error: "+err.Error(),
		)
		return
	}

	// If settings were explicitly specified, update the project with them.
	// Save the original create result so we can persist state even if the
	// settings update fails (preventing orphaned resources).
	// Note: check both IsNull and IsUnknown -- during Create, Optional+Computed
	// attributes that the user did not set will be Unknown (not Null).
	comparisonKeySet := !plan.ComparisonKey.IsNull() && !plan.ComparisonKey.IsUnknown()
	baselineSet := !plan.BaselineExperimentID.IsNull() && !plan.BaselineExperimentID.IsUnknown()
	if comparisonKeySet || baselineSet {
		updateReq := client.UpdateProjectRequest{
			Settings: &client.ProjectSettings{
				ComparisonKey:        tfStringToPtr(plan.ComparisonKey),
				BaselineExperimentID: tfStringToPtr(plan.BaselineExperimentID),
			},
		}

		_, updateErr := r.client.UpdateProject(ctx, created.ID, updateReq)
		if updateErr != nil {
			// Save the state from the create call so Terraform can track the resource.
			mapProjectToModel(created, &plan)
			resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
			resp.Diagnostics.AddError(
				"Error Setting Braintrust Project Settings",
				"Project was created (ID: "+created.ID+") but settings could not be applied: "+updateErr.Error(),
			)
			return
		}
	}

	// Re-read the project to get the full state -- the create/update
	// responses may not include all fields (e.g., description).
	project, err := r.client.GetProject(ctx, created.ID)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Reading Braintrust Project After Create",
			"Project was created (ID: "+created.ID+") but could not be read: "+err.Error(),
		)
		return
	}

	mapProjectToModel(project, &plan)

	tflog.Debug(ctx, "Created project", map[string]any{
		"id": plan.ID.ValueString(),
	})

	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

// Read refreshes the Terraform state with the latest data.
func (r *projectResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state projectModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !requireClient(r.client, &resp.Diagnostics) {
		return
	}

	tflog.Debug(ctx, "Reading project", map[string]any{
		"id": state.ID.ValueString(),
	})

	project, err := r.client.GetProject(ctx, state.ID.ValueString())
	if err != nil {
		if client.IsNotFoundError(err) {
			tflog.Warn(ctx, "Project not found, removing from state", map[string]any{
				"id": state.ID.ValueString(),
			})
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Error Reading Braintrust Project",
			"Could not read project ID "+state.ID.ValueString()+": "+err.Error(),
		)
		return
	}

	// Map response to model.
	mapProjectToModel(project, &state)

	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

// Update updates the resource and sets the updated Terraform state on success.
func (r *projectResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan projectModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var state projectModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if !requireClient(r.client, &resp.Diagnostics) {
		return
	}

	// Build the update request.
	name := plan.Name.ValueString()
	updateReq := client.UpdateProjectRequest{
		Name: &name,
	}

	// Handle description: send value if set, send empty string to clear if removed.
	if !plan.Description.IsNull() && !plan.Description.IsUnknown() {
		desc := plan.Description.ValueString()
		updateReq.Description = &desc
	} else if plan.Description.IsNull() && !state.Description.IsNull() {
		empty := ""
		updateReq.Description = &empty
	}

	// Only send settings if at least one setting field changed. Sending an empty
	// settings object {"settings": {}} to the PATCH endpoint can clobber other
	// fields on the server side.
	var settings client.ProjectSettings
	settingsChanged := false

	if !plan.ComparisonKey.IsNull() && !plan.ComparisonKey.IsUnknown() {
		key := plan.ComparisonKey.ValueString()
		settings.ComparisonKey = &key
		settingsChanged = true
	} else if plan.ComparisonKey.IsNull() && !state.ComparisonKey.IsNull() {
		empty := ""
		settings.ComparisonKey = &empty
		settingsChanged = true
	}
	if !plan.BaselineExperimentID.IsNull() && !plan.BaselineExperimentID.IsUnknown() {
		baseID := plan.BaselineExperimentID.ValueString()
		settings.BaselineExperimentID = &baseID
		settingsChanged = true
	} else if plan.BaselineExperimentID.IsNull() && !state.BaselineExperimentID.IsNull() {
		empty := ""
		settings.BaselineExperimentID = &empty
		settingsChanged = true
	}
	if settingsChanged {
		updateReq.Settings = &settings
	}

	tflog.Debug(ctx, "Updating project", map[string]any{
		"id": state.ID.ValueString(),
	})

	project, err := r.client.UpdateProject(ctx, state.ID.ValueString(), updateReq)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Updating Braintrust Project",
			"Could not update project ID "+state.ID.ValueString()+": "+err.Error(),
		)
		return
	}

	// Map response to model.
	mapProjectToModel(project, &plan)

	tflog.Debug(ctx, "Updated project", map[string]any{
		"id": plan.ID.ValueString(),
	})

	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

// Delete deletes the resource and removes the Terraform state on success.
func (r *projectResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state projectModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !requireClient(r.client, &resp.Diagnostics) {
		return
	}

	tflog.Debug(ctx, "Deleting project", map[string]any{
		"id": state.ID.ValueString(),
	})

	err := r.client.DeleteProject(ctx, state.ID.ValueString())
	if err != nil {
		if client.IsNotFoundError(err) {
			return
		}
		resp.Diagnostics.AddError(
			"Error Deleting Braintrust Project",
			"Could not delete project ID "+state.ID.ValueString()+": "+err.Error(),
		)
		return
	}

	tflog.Debug(ctx, "Deleted project", map[string]any{
		"id": state.ID.ValueString(),
	})
}

// ImportState imports an existing resource into Terraform state.
func (r *projectResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
