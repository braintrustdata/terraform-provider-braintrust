package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"github.com/braintrustdata/terraform-provider-braintrust/internal/client"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// Valid project score types from the Braintrust API.
var projectScoreTypes = []string{
	"slider",
	"categorical",
	"weighted",
	"minimum",
	"maximum",
	"online",
	"free-form",
}

var (
	_ resource.Resource                = &projectScoreResource{}
	_ resource.ResourceWithConfigure   = &projectScoreResource{}
	_ resource.ResourceWithImportState = &projectScoreResource{}
)

// NewProjectScoreResource returns a new project score resource.
func NewProjectScoreResource() resource.Resource {
	return &projectScoreResource{}
}

type projectScoreResource struct {
	client *client.Client
}

func (r *projectScoreResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_project_score"
}

func (r *projectScoreResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Version:     0,
		Description: "Manages a Braintrust project score.",
		MarkdownDescription: "Manages a Braintrust project score. Scores are user-configured metrics " +
			"that can be manually labeled through the UI or computed programmatically. " +
			"Use `categories_json` for categorical categories, weighted-score weights, or " +
			"minimum-score inputs, and `config_json` for score options. " +
			"Changing `score_type` replaces the score and assigns it a new ID.",
		Attributes: map[string]schema.Attribute{
			"id":      computedStringAttribute("Unique identifier for the project score (UUID)."),
			"user_id": computedStringAttribute("Identifies the user who created the project score."),
			"created": computedStringAttribute("Date of project score creation."),
			"project_id": schema.StringAttribute{
				Required:    true,
				Description: "Unique identifier for the project that the score belongs to.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"name": schema.StringAttribute{
				Required:    true,
				Description: "Name of the project score.",
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"description": optionalComputedStringAttribute("Textual description of the project score."),
			"score_type": schema.StringAttribute{
				Required:    true,
				Description: "The type of the configured score. Changing it replaces the score and assigns a new ID.",
				// PATCH validates the full type-specific score shape, and an
				// in-place type change can invalidate existing category/config data.
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					stringvalidator.OneOf(projectScoreTypes...),
				},
			},
			"categories_json": optionalComputedStringAttribute("JSON-encoded score categories, weights, or included score names, depending on score_type. A JSON null is sent to the API, which may reject it for score types requiring categories."),
			"config_json":     optionalComputedStringAttribute("JSON-encoded score configuration (for example multi_select, visibility, online, condition, and object_types). Braintrust does not clear this field on PATCH; replace the score to remove an existing configuration."),
		},
	}
}

func (r *projectScoreResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = clientFromProviderData(req.ProviderData, &resp.Diagnostics)
}

func (r *projectScoreResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan projectScoreModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !requireClient(r.client, &resp.Diagnostics) {
		return
	}

	createReq := client.CreateProjectScoreRequest{
		ProjectID:   plan.ProjectID.ValueString(),
		Name:        plan.Name.ValueString(),
		Description: tfStringToPtr(plan.Description),
		ScoreType:   plan.ScoreType.ValueString(),
	}
	var err error
	createReq.Categories, err = projectScoreJSONInput(plan.CategoriesJSON, "categories_json")
	if err != nil {
		resp.Diagnostics.AddError("Invalid Score Categories", err.Error())
		return
	}
	createReq.Config, err = projectScoreJSONInput(plan.ConfigJSON, "config_json")
	if err != nil {
		resp.Diagnostics.AddError("Invalid Score Config", err.Error())
		return
	}

	tflog.Debug(ctx, "Creating project score", map[string]any{
		"project_id": createReq.ProjectID,
		"name":       createReq.Name,
		"score_type": createReq.ScoreType,
	})

	created, err := r.client.CreateProjectScore(ctx, createReq)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Creating Braintrust Project Score",
			"Could not create project score, unexpected error: "+err.Error(),
		)
		return
	}

	// Re-read to get the full state.
	score, err := r.client.GetProjectScore(ctx, created.ID)
	if err != nil {
		mapProjectScoreToModel(created, &plan)
		resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
		resp.Diagnostics.AddError(
			"Error Reading Braintrust Project Score After Create",
			"Score was created (ID: "+created.ID+") but could not be read: "+err.Error(),
		)
		return
	}

	mapProjectScoreToModel(score, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *projectScoreResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state projectScoreModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !requireClient(r.client, &resp.Diagnostics) {
		return
	}

	score, err := r.client.GetProjectScore(ctx, state.ID.ValueString())
	if err != nil {
		if client.IsNotFoundError(err) {
			tflog.Warn(ctx, "Project score not found, removing from state", map[string]any{
				"id": state.ID.ValueString(),
			})
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Error Reading Braintrust Project Score",
			"Could not read project score ID "+state.ID.ValueString()+": "+err.Error(),
		)
		return
	}

	mapProjectScoreToModel(score, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *projectScoreResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan projectScoreModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var state projectScoreModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !requireClient(r.client, &resp.Diagnostics) {
		return
	}

	name := plan.Name.ValueString()
	updateReq := client.UpdateProjectScoreRequest{
		Name: &name,
	}
	categories, err := projectScoreJSONInput(plan.CategoriesJSON, "categories_json")
	if err != nil {
		resp.Diagnostics.AddError("Invalid Score Categories", err.Error())
		return
	}
	if categories != nil && !scoreJSONEqual(plan.CategoriesJSON, state.CategoriesJSON) {
		updateReq.Categories = &categories
	}
	config, err := projectScoreJSONInput(plan.ConfigJSON, "config_json")
	if err != nil {
		resp.Diagnostics.AddError("Invalid Score Config", err.Error())
		return
	}
	if config != nil && !scoreJSONEqual(plan.ConfigJSON, state.ConfigJSON) {
		updateReq.Config = &config
	}

	if !plan.Description.IsNull() && !plan.Description.IsUnknown() {
		desc := plan.Description.ValueString()
		updateReq.Description = &desc
	} else if plan.Description.IsNull() && !state.Description.IsNull() {
		empty := ""
		updateReq.Description = &empty
	}

	score, err := r.client.UpdateProjectScore(ctx, state.ID.ValueString(), updateReq)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Updating Braintrust Project Score",
			"Could not update project score ID "+state.ID.ValueString()+": "+err.Error(),
		)
		return
	}

	mapProjectScoreToModel(score, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *projectScoreResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state projectScoreModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !requireClient(r.client, &resp.Diagnostics) {
		return
	}

	err := r.client.DeleteProjectScore(ctx, state.ID.ValueString())
	if err != nil {
		if client.IsNotFoundError(err) {
			return
		}
		resp.Diagnostics.AddError(
			"Error Deleting Braintrust Project Score",
			"Could not delete project score ID "+state.ID.ValueString()+": "+err.Error(),
		)
	}
}

func (r *projectScoreResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func projectScoreJSONInput(value types.String, field string) (json.RawMessage, error) {
	if value.IsNull() || value.IsUnknown() {
		return nil, nil
	}
	raw := json.RawMessage(value.ValueString())
	if !json.Valid(raw) {
		return nil, fmt.Errorf("%s must be valid JSON", field)
	}
	if field == "config_json" && bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil, fmt.Errorf("%s cannot be JSON null: Braintrust does not clear score configuration on PATCH; replace the score to remove an existing configuration", field)
	}
	return raw, nil
}

func scoreJSONEqual(a, b types.String) bool {
	if a.IsNull() || a.IsUnknown() || b.IsNull() || b.IsUnknown() {
		return a.Equal(b)
	}
	return jsonRawToTFString(json.RawMessage(a.ValueString())).Equal(jsonRawToTFString(json.RawMessage(b.ValueString())))
}
