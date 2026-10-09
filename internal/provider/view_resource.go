package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/braintrustdata/terraform-provider-braintrust/internal/client"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource                = &viewResource{}
	_ resource.ResourceWithConfigure   = &viewResource{}
	_ resource.ResourceWithImportState = &viewResource{}
)

type viewResource struct{ client *client.Client }

type viewModel struct {
	ID           types.String `tfsdk:"id"`
	ObjectType   types.String `tfsdk:"object_type"`
	ObjectID     types.String `tfsdk:"object_id"`
	ViewType     types.String `tfsdk:"view_type"`
	Name         types.String `tfsdk:"name"`
	Description  types.String `tfsdk:"description"`
	Starred      types.Bool   `tfsdk:"starred"`
	ViewDataJSON types.String `tfsdk:"view_data_json"`
	OptionsJSON  types.String `tfsdk:"options_json"`
	Created      types.String `tfsdk:"created"`
	UpdatedAt    types.String `tfsdk:"updated_at"`
	UserID       types.String `tfsdk:"user_id"`
}

func NewViewResource() resource.Resource { return &viewResource{} }
func (r *viewResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_view"
}
func (r *viewResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = clientFromProviderData(req.ProviderData, &resp.Diagnostics)
}

func (r *viewResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description:         "Manages a Braintrust saved view or monitor dashboard, including its charts.",
		MarkdownDescription: "Manages a Braintrust saved view or monitor dashboard. A dashboard is a view with `view_type = \"monitor\"`; its charts are stored in `view_data_json.custom_charts` and share the view lifecycle. JSON values must be objects. Braintrust deep-merges `options_json` on PATCH, so removing an existing option key requires replacing the view. Import with `object_type/object_id/view_id` because Braintrust requires the containing object when reading a view.",
		Attributes: map[string]schema.Attribute{
			"id":             computedStringAttribute("Unique identifier for the view (UUID)."),
			"object_type":    schema.StringAttribute{Required: true, Description: "Type of the containing object, commonly project or org_project.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}, Validators: []validator.String{stringvalidator.OneOf("organization", "project", "experiment", "dataset", "prompt", "prompt_session", "group", "role", "org_member", "project_log", "org_project", "org_audit_logs", "project_group", "project_group_projects", "ai_secret", "org_ai_secret", "org_account")}},
			"object_id":      schema.StringAttribute{Required: true, Description: "ID of the containing object. For org_project dashboards, this is the organization ID; set options_json.options.projectId to the project ID.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"view_type":      schema.StringAttribute{Required: true, Description: "Braintrust view type. Use monitor for dashboards or logs for saved project log views.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}, Validators: []validator.String{stringvalidator.OneOf("projects", "experiments", "experiment", "playgrounds", "playground", "datasets", "dataset", "prompts", "parameters", "tools", "scorers", "classifiers", "logs", "monitor", "for_review_project_log", "for_review_experiments", "for_review_datasets")}},
			"name":           schema.StringAttribute{Required: true, Description: "Name of the view.", Validators: []validator.String{stringvalidator.LengthAtLeast(1)}},
			"description":    optionalComputedStringAttribute("Description of the view."),
			"starred":        schema.BoolAttribute{Optional: true, Computed: true, Description: "Whether the view is starred in the project."},
			"view_data_json": optionalComputedStringAttribute("JSON object containing search and/or custom_charts. custom_charts requires a version (currently 0.0.0), and may contain charts, groups, and layout."),
			"options_json":   optionalComputedStringAttribute("JSON object with app view options. Monitor dashboards use viewType=monitor and a nested options object. Existing keys cannot be removed by PATCH; replace the view to remove one."),
			"created":        computedStringAttribute("Date of creation."),
			"updated_at":     schema.StringAttribute{Computed: true, Description: "Date of last update."},
			"user_id":        computedStringAttribute("Creator's user ID."),
		},
	}
}

func viewJSONInput(value types.String, field string) (json.RawMessage, error) {
	if value.IsNull() || value.IsUnknown() {
		return nil, nil
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal([]byte(value.ValueString()), &object); err != nil || object == nil {
		return nil, fmt.Errorf("%s must be a JSON object", field)
	}
	return json.RawMessage(value.ValueString()), nil
}

func viewJSONState(remote json.RawMessage, existing types.String) types.String {
	if len(remote) == 0 || bytes.Equal(bytes.TrimSpace(remote), []byte("null")) {
		return types.StringNull()
	}
	return promptJSONState(remote, existing)
}

func mapViewToModel(view *client.View, model *viewModel) {
	model.ID = types.StringValue(view.ID)
	model.ObjectType = types.StringValue(view.ObjectType)
	model.ObjectID = types.StringValue(view.ObjectID)
	model.ViewType = types.StringValue(view.ViewType)
	model.Name = types.StringValue(view.Name)
	model.Description = stringPtrToTFValue(view.Description)
	if view.Starred == nil {
		model.Starred = types.BoolNull()
	} else {
		model.Starred = types.BoolValue(*view.Starred)
	}
	model.ViewDataJSON = viewJSONState(view.ViewData, model.ViewDataJSON)
	model.OptionsJSON = viewJSONState(view.Options, model.OptionsJSON)
	model.Created = stringPtrToTFValue(view.Created)
	model.UpdatedAt = stringPtrToTFValue(view.UpdatedAt)
	model.UserID = stringPtrToTFValue(view.UserID)
}

func (r *viewResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan viewModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() || !requireClient(r.client, &resp.Diagnostics) {
		return
	}
	viewData, err := viewJSONInput(plan.ViewDataJSON, "view_data_json")
	if err != nil {
		resp.Diagnostics.AddError("Invalid View Data", err.Error())
		return
	}
	options, err := viewJSONInput(plan.OptionsJSON, "options_json")
	if err != nil {
		resp.Diagnostics.AddError("Invalid View Options", err.Error())
		return
	}
	// POST signals an existing same-name view through x-bt-found-existing.
	// The client rejects that result rather than silently adopting the view.
	created, err := r.client.CreateView(ctx, client.CreateViewRequest{ObjectType: plan.ObjectType.ValueString(), ObjectID: plan.ObjectID.ValueString(), ViewType: plan.ViewType.ValueString(), Name: plan.Name.ValueString(), Description: tfStringToPtr(plan.Description), ViewData: viewData, Options: options})
	if err != nil {
		resp.Diagnostics.AddError("Error Creating Braintrust View", err.Error())
		return
	}
	// Starred is not accepted by POST. Set it after creation when requested.
	if !plan.Starred.IsNull() && !plan.Starred.IsUnknown() && plan.Starred.ValueBool() {
		starred := true
		_, err = r.client.UpdateView(ctx, created.ID, client.UpdateViewRequest{ObjectType: created.ObjectType, ObjectID: created.ObjectID, Starred: &starred})
		if err != nil {
			mapViewToModel(created, &plan)
			resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
			resp.Diagnostics.AddError("Error Starring Created View", "View was created (ID: "+created.ID+") but could not be starred: "+err.Error())
			return
		}
	}
	view, err := r.client.GetView(ctx, created.ID, created.ObjectType, created.ObjectID)
	if err != nil {
		mapViewToModel(created, &plan)
		resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
		resp.Diagnostics.AddError("Error Reading Created View", "View was created (ID: "+created.ID+") but could not be read: "+err.Error())
		return
	}
	mapViewToModel(view, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *viewResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state viewModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() || !requireClient(r.client, &resp.Diagnostics) {
		return
	}
	view, err := r.client.GetView(ctx, state.ID.ValueString(), state.ObjectType.ValueString(), state.ObjectID.ValueString())
	if client.IsNotFoundError(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Error Reading Braintrust View", err.Error())
		return
	}
	mapViewToModel(view, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *viewResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state viewModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() || !requireClient(r.client, &resp.Diagnostics) {
		return
	}
	update := client.UpdateViewRequest{ObjectType: state.ObjectType.ValueString(), ObjectID: state.ObjectID.ValueString()}
	if !plan.Name.Equal(state.Name) {
		name := plan.Name.ValueString()
		update.Name = &name
	}
	if !plan.Description.Equal(state.Description) {
		description := plan.Description.ValueString()
		update.Description = &description
	}
	if !plan.Starred.Equal(state.Starred) && !plan.Starred.IsNull() {
		starred := plan.Starred.ValueBool()
		update.Starred = &starred
	}
	if !scoreJSONEqual(plan.ViewDataJSON, state.ViewDataJSON) {
		data, err := viewJSONInput(plan.ViewDataJSON, "view_data_json")
		if err != nil {
			resp.Diagnostics.AddError("Invalid View Data", err.Error())
			return
		}
		if data == nil {
			resp.Diagnostics.AddError("Cannot Clear View Data", "Set view_data_json to jsonencode({}) to clear it, or replace the view.")
			return
		}
		update.ViewData = &data
	}
	if !scoreJSONEqual(plan.OptionsJSON, state.OptionsJSON) {
		options, err := viewJSONInput(plan.OptionsJSON, "options_json")
		if err != nil {
			resp.Diagnostics.AddError("Invalid View Options", err.Error())
			return
		}
		if options == nil || viewJSONRemovesKeys(state.OptionsJSON.ValueString(), string(options)) {
			resp.Diagnostics.AddError("Cannot Remove View Options", "Braintrust merges options on PATCH. Keep existing keys or replace the view to remove them.")
			return
		}
		update.Options = &options
	}
	view, err := r.client.UpdateView(ctx, state.ID.ValueString(), update)
	if err != nil {
		resp.Diagnostics.AddError("Error Updating Braintrust View", err.Error())
		return
	}
	mapViewToModel(view, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func viewJSONRemovesKeys(oldJSON, newJSON string) bool {
	var oldValue, newValue map[string]any
	if json.Unmarshal([]byte(oldJSON), &oldValue) != nil || json.Unmarshal([]byte(newJSON), &newValue) != nil {
		return false
	}
	return removesObjectKeys(oldValue, newValue)
}

func removesObjectKeys(oldValue, newValue map[string]any) bool {
	for key, oldChild := range oldValue {
		newChild, ok := newValue[key]
		if !ok {
			return true
		}
		if oldObject, ok := oldChild.(map[string]any); ok {
			if newObject, ok := newChild.(map[string]any); ok && removesObjectKeys(oldObject, newObject) {
				return true
			}
		}
	}
	return false
}

func (r *viewResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state viewModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() || !requireClient(r.client, &resp.Diagnostics) {
		return
	}
	if err := r.client.DeleteView(ctx, state.ID.ValueString(), state.ObjectType.ValueString(), state.ObjectID.ValueString()); err != nil && !client.IsNotFoundError(err) {
		resp.Diagnostics.AddError("Error Deleting Braintrust View", err.Error())
	}
}

func (r *viewResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.Split(req.ID, "/")
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		resp.Diagnostics.AddError("Invalid View Import ID", "Use object_type/object_id/view_id because the Braintrust API requires the containing object when reading a view.")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("object_type"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("object_id"), parts[1])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), parts[2])...)
}
