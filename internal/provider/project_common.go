package provider

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/braintrustdata/braintrust-sdk-go/openapi"
	"github.com/braintrustdata/terraform-provider-braintrust/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// projectModel is the shared data model for both the project resource and data source.
type projectModel struct {
	ID                   types.String `tfsdk:"id"`
	OrgID                types.String `tfsdk:"org_id"`
	Name                 types.String `tfsdk:"name"`
	Description          types.String `tfsdk:"description"`
	Created              types.String `tfsdk:"created"`
	DeletedAt            types.String `tfsdk:"deleted_at"`
	UserID               types.String `tfsdk:"user_id"`
	ComparisonKey        types.String `tfsdk:"comparison_key"`
	BaselineExperimentID types.String `tfsdk:"baseline_experiment_id"`
}

// mapProjectToModel maps an SDK project to the shared Terraform model.
// Both project and model must be non-nil.
func mapProjectToModel(project *openapi.Project, model *projectModel) {
	model.ID = types.StringValue(project.Id.String())
	model.OrgID = types.StringValue(project.OrgId.String())
	model.Name = types.StringValue(project.Name)

	model.Description = nullableStringToTF(project.Description)
	model.Created = nullableTimeToTF(project.Created, model.Created)
	model.DeletedAt = nullableTimeToTF(project.DeletedAt, model.DeletedAt)
	model.UserID = nullableUUIDToTF(project.UserId)

	// Map settings fields.
	if nullableIsSet(project.Settings) {
		settings := project.Settings.MustGet()
		model.ComparisonKey = nullableStringToTF(settings.ComparisonKey)
		model.BaselineExperimentID = nullableUUIDToTF(settings.BaselineExperimentId)
	} else {
		model.ComparisonKey = types.StringNull()
		model.BaselineExperimentID = types.StringNull()
	}
}

// getProject reads a project along with its raw settings. The raw settings
// keep fields newer than the SDK's models, so a settings PATCH (which replaces
// the whole object) can send them back unchanged.
func getProject(ctx context.Context, c *client.Client, id string) (*openapi.Project, map[string]json.RawMessage, error) {
	resp, err := c.API.GetProjectIdWithResponse(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	project, err := client.JSON(resp.JSON200)
	if err != nil {
		return nil, nil, err
	}
	var raw struct {
		Settings map[string]json.RawMessage `json:"settings"`
	}
	if err := json.Unmarshal(resp.Body, &raw); err != nil {
		return nil, nil, fmt.Errorf("failed to decode project settings: %w", err)
	}
	return project, raw.Settings, nil
}

// updateProject PATCHes a project. Settings are sent as raw JSON rather than
// through the SDK model so unmanaged fields the model does not know survive.
func updateProject(ctx context.Context, c *client.Client, id string, patch openapi.PatchProject, settings map[string]json.RawMessage) (*openapi.Project, error) {
	var raw map[string]json.RawMessage
	if settings != nil {
		encoded, err := json.Marshal(settings)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal project settings: %w", err)
		}
		raw = map[string]json.RawMessage{"settings": encoded}
	}
	body, err := client.JSONBody(patch, raw)
	if err != nil {
		return nil, err
	}
	resp, err := client.Write(c.API.PatchProjectIdWithBodyWithResponse(ctx, id, client.JSONContentType, body))
	if err != nil {
		return nil, err
	}
	return client.Written(resp.JSON200)
}
