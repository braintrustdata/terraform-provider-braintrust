package provider

import (
	"context"
	"encoding/json"

	"github.com/braintrustdata/braintrust-sdk-go/openapi"
	"github.com/braintrustdata/terraform-provider-braintrust/internal/client"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/oapi-codegen/nullable"
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
		Version:             0,
		Description:         "Manages a Braintrust project.",
		MarkdownDescription: "Manages a Braintrust [project](https://docs.braintrust.dev/admin/projects). Projects are the top-level organizational unit in Braintrust, containing experiments, datasets, prompts, and more.",
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
	createReq := openapi.CreateProject{
		Name:        plan.Name.ValueString(),
		Description: tfStringToNullable(plan.Description),
	}

	tflog.Debug(ctx, "Creating project", map[string]any{
		"name": createReq.Name,
	})

	createResp, err := client.Write(r.client.API.PostProjectWithResponse(ctx, createReq))
	var created *openapi.Project
	if err == nil {
		created, err = client.Created(createResp.HTTPResponse, createResp.JSON200, "project")
	}
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Creating Braintrust Project",
			"Could not create project, unexpected error: "+err.Error(),
		)
		return
	}

	createdID := created.Id.String()
	project, settings, err := getProject(ctx, r.client, createdID)
	if err != nil {
		mapProjectToModel(created, &plan)
		resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
		resp.Diagnostics.AddError("Error Reading Braintrust Project After Create", "Project was created (ID: "+createdID+") but could not be read: "+err.Error())
		return
	}

	// Project settings are replaced by the API. Keep server-owned settings
	// when applying either of the two fields managed by this resource.
	comparisonKeySet := !plan.ComparisonKey.IsNull() && !plan.ComparisonKey.IsUnknown()
	baselineSet := !plan.BaselineExperimentID.IsNull() && !plan.BaselineExperimentID.IsUnknown()
	if comparisonKeySet || baselineSet {
		merged := mergeProjectSettings(settings, &plan, comparisonKeySet, baselineSet)

		_, updateErr := updateProject(ctx, r.client, createdID, openapi.PatchProject{}, merged)
		if updateErr != nil {
			mapProjectToModel(project, &plan)
			resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
			resp.Diagnostics.AddError(
				"Error Setting Braintrust Project Settings",
				"Project was created (ID: "+createdID+") but settings could not be applied: "+updateErr.Error(),
			)
			return
		}
		project, _, err = getProject(ctx, r.client, createdID)
		if err != nil {
			mapProjectToModel(created, &plan)
			resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
			resp.Diagnostics.AddError("Error Reading Braintrust Project After Settings Update", "Project was created (ID: "+createdID+") but could not be read: "+err.Error())
			return
		}
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

	project, _, err := getProject(ctx, r.client, state.ID.ValueString())
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
	// Description is sent when set and cleared with an empty string when removed.
	updateReq := openapi.PatchProject{
		Name:        nullable.NewNullableWithValue(plan.Name.ValueString()),
		Description: patchString(plan.Description, state.Description),
	}

	var settings map[string]json.RawMessage

	comparisonChanged := !plan.ComparisonKey.IsUnknown() && !plan.ComparisonKey.Equal(state.ComparisonKey)
	baselineChanged := !plan.BaselineExperimentID.IsUnknown() && !plan.BaselineExperimentID.Equal(state.BaselineExperimentID)
	if comparisonChanged || baselineChanged {
		_, current, err := getProject(ctx, r.client, state.ID.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Error Reading Braintrust Project Settings", "Could not preserve current project settings: "+err.Error())
			return
		}
		settings = mergeProjectSettings(current, &plan, comparisonChanged, baselineChanged)
	}

	tflog.Debug(ctx, "Updating project", map[string]any{
		"id": state.ID.ValueString(),
	})

	project, err := updateProject(ctx, r.client, state.ID.ValueString(), updateReq, settings)
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

func mergeProjectSettings(current map[string]json.RawMessage, plan *projectModel, comparisonChanged, baselineChanged bool) map[string]json.RawMessage {
	settings := make(map[string]json.RawMessage, len(current)+2)
	for name, value := range current {
		settings[name] = value
	}
	if comparisonChanged {
		settings["comparison_key"] = projectSettingValue(plan.ComparisonKey.IsNull(), plan.ComparisonKey.ValueString())
	}
	if baselineChanged {
		settings["baseline_experiment_id"] = projectSettingValue(plan.BaselineExperimentID.IsNull(), plan.BaselineExperimentID.ValueString())
	}
	return settings
}

func projectSettingValue(isNull bool, value string) json.RawMessage {
	if isNull {
		return json.RawMessage("null")
	}
	encoded, _ := json.Marshal(value)
	return encoded
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

	_, err := client.Write(r.client.API.DeleteProjectIdWithResponse(ctx, state.ID.ValueString()))
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
