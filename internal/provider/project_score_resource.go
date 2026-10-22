package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"github.com/braintrustdata/braintrust-sdk-go/openapi"
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
	"github.com/oapi-codegen/nullable"
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

	projectID, err := parseUUID(plan.ProjectID, "project_id")
	if err != nil {
		resp.Diagnostics.AddAttributeError(path.Root("project_id"), "Invalid Project ID", err.Error())
		return
	}
	createReq := openapi.CreateProjectScore{
		ProjectId:   projectID,
		Name:        plan.Name.ValueString(),
		Description: tfStringToNullable(plan.Description),
		ScoreType:   openapi.ProjectScoreType(plan.ScoreType.ValueString()),
	}
	categories, err := projectScoreJSONInput(plan.CategoriesJSON, "categories_json")
	if err != nil {
		resp.Diagnostics.AddError("Invalid Score Categories", err.Error())
		return
	}
	config, err := projectScoreJSONInput(plan.ConfigJSON, "config_json")
	if err != nil {
		resp.Diagnostics.AddError("Invalid Score Config", err.Error())
		return
	}
	body, err := client.JSONBody(createReq, map[string]json.RawMessage{"categories": categories, "config": config})
	if err != nil {
		resp.Diagnostics.AddError("Error Creating Braintrust Project Score", err.Error())
		return
	}

	tflog.Debug(ctx, "Creating project score", map[string]any{
		"project_id": plan.ProjectID.ValueString(),
		"name":       createReq.Name,
		"score_type": createReq.ScoreType,
	})

	createResp, err := client.Write(r.client.API.PostProjectScoreWithBodyWithResponse(ctx, client.JSONContentType, body))
	var created *projectScore
	if err == nil {
		var createdBody *openapi.ProjectScore
		createdBody, err = client.Created(createResp.HTTPResponse, createResp.JSON200, "project score")
		if err == nil {
			created, err = client.DecodeRaw(createdBody, createResp.Body)
		}
	}
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Creating Braintrust Project Score",
			"Could not create project score, unexpected error: "+err.Error(),
		)
		return
	}

	// Re-read to get the full state.
	score, err := getProjectScore(ctx, r.client, created.Value.Id.String())
	if err != nil {
		mapProjectScoreToModel(created, &plan)
		resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
		resp.Diagnostics.AddError(
			"Error Reading Braintrust Project Score After Create",
			"Score was created (ID: "+created.Value.Id.String()+") but could not be read: "+err.Error(),
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

	score, err := getProjectScore(ctx, r.client, state.ID.ValueString())
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

	updateReq := openapi.PatchProjectScore{
		Name:        nullable.NewNullableWithValue(plan.Name.ValueString()),
		Description: patchString(plan.Description, state.Description),
	}
	raw := map[string]json.RawMessage{}
	if !scoreJSONEqual(plan.CategoriesJSON, state.CategoriesJSON) {
		categories, err := projectScoreJSONInput(plan.CategoriesJSON, "categories_json")
		if err != nil {
			resp.Diagnostics.AddError("Invalid Score Categories", err.Error())
			return
		}
		raw["categories"] = categories
	}
	if !scoreJSONEqual(plan.ConfigJSON, state.ConfigJSON) {
		config, err := projectScoreJSONInput(plan.ConfigJSON, "config_json")
		if err != nil {
			resp.Diagnostics.AddError("Invalid Score Config", err.Error())
			return
		}
		raw["config"] = config
	}
	body, err := client.JSONBody(updateReq, raw)
	if err != nil {
		resp.Diagnostics.AddError("Error Updating Braintrust Project Score", err.Error())
		return
	}

	updateResp, err := client.Write(r.client.API.PatchProjectScoreIdWithBodyWithResponse(ctx, state.ID.ValueString(), client.JSONContentType, body))
	var score *projectScore
	if err == nil {
		score, err = client.DecodeRaw(updateResp.JSON200, updateResp.Body)
	}
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

	_, err := client.Write(r.client.API.DeleteProjectScoreIdWithResponse(ctx, state.ID.ValueString()))
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
