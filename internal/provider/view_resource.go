package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"

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
	"github.com/oapi-codegen/nullable"
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
		MarkdownDescription: "Manages a Braintrust saved view or monitor dashboard. A dashboard is a view with `view_type = \"monitor\"`; its charts are stored in `view_data_json.custom_charts` and share the view lifecycle. JSON values must be objects. Braintrust deep-merges `options_json` on PATCH, so removing an existing option key requires replacing the view.",
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
	return jsonState(remote, existing)
}

// view keeps view_data and options as raw JSON, which Terraform passes
// through verbatim instead of round-tripping them through SDK models.
type view = client.Raw[openapi.View]

func mapViewToModel(raw *view, model *viewModel) {
	v := raw.Value
	model.ID = types.StringValue(v.Id.String())
	model.ObjectType = types.StringValue(string(v.ObjectType))
	model.ObjectID = types.StringValue(v.ObjectId.String())
	model.ViewType = types.StringValue(string(v.ViewType.GetOrEmpty()))
	model.Name = types.StringValue(v.Name)
	model.Description = nullableStringToTF(v.Description)
	if v.Starred == nil {
		model.Starred = types.BoolNull()
	} else {
		model.Starred = types.BoolValue(*v.Starred)
	}
	model.ViewDataJSON = viewJSONState(raw.Fields["view_data"], model.ViewDataJSON)
	model.OptionsJSON = viewJSONState(raw.Fields["options"], model.OptionsJSON)
	model.Created = nullableTimeToTF(v.Created, model.Created)
	model.UpdatedAt = nullableTimeToTF(v.UpdatedAt, model.UpdatedAt)
	model.UserID = nullableUUIDToTF(v.UserId)
}

// Views are scoped to an object, so every request names it.
func getView(ctx context.Context, c *client.Client, id, objectType, objectID string) (*view, error) {
	resp, err := c.API.GetViewIdWithResponse(ctx, id, &openapi.GetViewIdParams{
		ObjectType: openapi.AclObjectType(objectType),
		ObjectId:   objectID,
	})
	if err != nil {
		return nil, err
	}
	return client.DecodeRaw(resp.JSON200, resp.Body)
}

func patchView(ctx context.Context, c *client.Client, id string, patch openapi.PatchView, raw map[string]json.RawMessage) (*view, error) {
	body, err := client.JSONBody(patch, raw)
	if err != nil {
		return nil, err
	}
	resp, err := client.Write(c.API.PatchViewIdWithBodyWithResponse(ctx, id, client.JSONContentType, body))
	if err != nil {
		return nil, err
	}
	return client.WrittenRaw(resp.JSON200, resp.Body)
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
	objectID, err := parseUUID(plan.ObjectID, "object_id")
	if err != nil {
		resp.Diagnostics.AddAttributeError(path.Root("object_id"), "Invalid Object ID", err.Error())
		return
	}
	body, err := client.JSONBody(openapi.CreateView{
		ObjectType:  openapi.AclObjectType(plan.ObjectType.ValueString()),
		ObjectId:    objectID,
		ViewType:    nullable.NewNullableWithValue(openapi.CreateViewViewType(plan.ViewType.ValueString())),
		Name:        plan.Name.ValueString(),
		Description: tfStringToNullable(plan.Description),
	}, map[string]json.RawMessage{"view_data": viewData, "options": options})
	if err != nil {
		resp.Diagnostics.AddError("Error Creating Braintrust View", err.Error())
		return
	}
	// POST signals an existing same-name view through x-bt-found-existing.
	// Reject that result rather than silently adopting the view.
	createResp, err := client.Write(r.client.API.PostViewWithBodyWithResponse(ctx, client.JSONContentType, body))
	var created *view
	if err == nil {
		existing := fmt.Sprintf("view %q in %s %s", plan.Name.ValueString(), plan.ObjectType.ValueString(), plan.ObjectID.ValueString())
		var createdBody *openapi.View
		createdBody, err = client.Created(createResp.HTTPResponse, createResp.JSON200, existing)
		if err == nil {
			created, err = client.WrittenRaw(createdBody, createResp.Body)
		}
	}
	if err != nil {
		resp.Diagnostics.AddError("Error Creating Braintrust View", err.Error())
		return
	}
	createdID := created.Value.Id.String()
	// Starred is not accepted by POST. Set it after creation when requested.
	if !plan.Starred.IsNull() && !plan.Starred.IsUnknown() && plan.Starred.ValueBool() {
		starred := true
		_, err = patchView(ctx, r.client, createdID, openapi.PatchView{ObjectType: created.Value.ObjectType, ObjectId: created.Value.ObjectId, Starred: &starred}, nil)
		if err != nil {
			mapViewToModel(created, &plan)
			resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
			resp.Diagnostics.AddError("Error Starring Created View", "View was created (ID: "+createdID+") but could not be starred: "+err.Error())
			return
		}
	}
	view, err := getView(ctx, r.client, createdID, string(created.Value.ObjectType), created.Value.ObjectId.String())
	if err != nil {
		mapViewToModel(created, &plan)
		resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
		resp.Diagnostics.AddError("Error Reading Created View", "View was created (ID: "+createdID+") but could not be read: "+err.Error())
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
	view, err := getView(ctx, r.client, state.ID.ValueString(), state.ObjectType.ValueString(), state.ObjectID.ValueString())
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
	objectID, err := parseUUID(state.ObjectID, "object_id")
	if err != nil {
		resp.Diagnostics.AddAttributeError(path.Root("object_id"), "Invalid Object ID", err.Error())
		return
	}
	update := openapi.PatchView{ObjectType: openapi.AclObjectType(state.ObjectType.ValueString()), ObjectId: objectID}
	raw := map[string]json.RawMessage{}
	if !plan.Name.Equal(state.Name) {
		update.Name = nullable.NewNullableWithValue(plan.Name.ValueString())
	}
	if !plan.Description.Equal(state.Description) {
		update.Description = nullable.NewNullableWithValue(plan.Description.ValueString())
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
		raw["view_data"] = data
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
		raw["options"] = options
	}
	view, err := patchView(ctx, r.client, state.ID.ValueString(), update, raw)
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
	objectID, err := parseUUID(state.ObjectID, "object_id")
	if err != nil {
		resp.Diagnostics.AddError("Error Deleting Braintrust View", err.Error())
		return
	}
	scope := openapi.DeleteView{ObjectType: openapi.AclObjectType(state.ObjectType.ValueString()), ObjectId: objectID}
	if _, err := client.Write(r.client.API.DeleteViewIdWithResponse(ctx, state.ID.ValueString(), scope)); err != nil && !client.IsNotFoundError(err) {
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
