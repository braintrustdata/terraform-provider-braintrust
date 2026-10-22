package provider

import (
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
	"github.com/oapi-codegen/nullable"
)

var (
	_ resource.Resource                = &functionResource{}
	_ resource.ResourceWithConfigure   = &functionResource{}
	_ resource.ResourceWithImportState = &functionResource{}
)

func NewFunctionResource() resource.Resource { return &functionResource{} }

type functionResource struct{ client *client.Client }

func (r *functionResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_function"
}

func (r *functionResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description:         "Manages a Braintrust function, such as a scorer, preprocessor, tool, or task.",
		MarkdownDescription: "Manages a Braintrust function through `/v1/function`. `function_data_json` is the API's tagged function-data object. This includes prompts, scorers, preprocessors, tools, and other supported function types.",
		Attributes: map[string]schema.Attribute{
			"id":      computedStringAttribute("Unique identifier for the function (UUID)."),
			"xact_id": schema.StringAttribute{Computed: true, Description: "Transaction ID of the function version."},
			"org_id":  computedStringAttribute("Organization ID."),
			"created": computedStringAttribute("Creation time."),
			"project_id": schema.StringAttribute{
				Required: true, Description: "Project containing the function.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"name": schema.StringAttribute{
				Required: true, Description: "Display name of the function.",
				Validators: []validator.String{stringvalidator.LengthAtLeast(1)},
			},
			"slug": schema.StringAttribute{
				Required: true, Description: "Unique identifier within the project.",
				Validators:    []validator.String{stringvalidator.LengthAtLeast(1)},
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"function_type": schema.StringAttribute{
				Optional: true, Computed: true,
				Description:   "Function type, for example scorer, preprocessor, tool, task, or llm. Omit for a prompt function to let the API choose its prompt type.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace(), stringplanmodifier.UseStateForUnknown()},
			},
			"function_data_json": schema.StringAttribute{
				Required:    true,
				Description: "JSON object describing the implementation. For inline code, use type=code with data.type=inline, runtime_context, and code. Global functions use type=global with a name and optional config.",
			},
			"prompt_data_json": optionalComputedStringAttribute("Optional JSON prompt configuration for LLM-backed functions."),
			"function_schema_json": schema.StringAttribute{
				Optional: true, Computed: true,
				Description:   "Optional JSON schema for function parameters and return value. Changes replace the function because PATCH does not accept this field.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace(), stringplanmodifier.UseStateForUnknown()},
			},
			"description":   optionalComputedStringAttribute("Textual description."),
			"tags":          schema.ListAttribute{Optional: true, Computed: true, ElementType: types.StringType, Description: "Tags for the function."},
			"metadata_json": schema.StringAttribute{Computed: true, Description: "Metadata object returned by the API as JSON."},
		},
	}
}

func (r *functionResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = clientFromProviderData(req.ProviderData, &resp.Diagnostics)
}

func (r *functionResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan functionModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() || !requireClient(r.client, &resp.Diagnostics) {
		return
	}
	functionData, err := functionJSONObject(plan.FunctionDataJSON, "function_data_json")
	if err != nil {
		resp.Diagnostics.AddError("Invalid Function Data", err.Error())
		return
	}
	promptData, err := functionJSONObject(plan.PromptDataJSON, "prompt_data_json")
	if err != nil {
		resp.Diagnostics.AddError("Invalid Prompt Data", err.Error())
		return
	}
	functionSchema, err := functionJSONObject(plan.FunctionSchemaJSON, "function_schema_json")
	if err != nil {
		resp.Diagnostics.AddError("Invalid Function Schema", err.Error())
		return
	}
	projectID, err := parseUUID(plan.ProjectID, "project_id")
	if err != nil {
		resp.Diagnostics.AddAttributeError(path.Root("project_id"), "Invalid Project ID", err.Error())
		return
	}
	createReq := openapi.CreateFunction{
		ProjectId: projectID, Name: plan.Name.ValueString(), Slug: plan.Slug.ValueString(),
		Description: tfStringToNullable(plan.Description), Tags: nullableSlice(stringListFromModel(plan.Tags)),
	}
	if functionType := tfStringToEnum[openapi.FunctionTypeEnumNullish](plan.FunctionType); functionType != nil {
		createReq.FunctionType = nullable.NewNullableWithValue(*functionType)
	}
	body, err := client.JSONBody(createReq, map[string]json.RawMessage{
		"function_data": functionData, "prompt_data": promptData, "function_schema": functionSchema,
	})
	if err != nil {
		resp.Diagnostics.AddError("Error Creating Braintrust Function", err.Error())
		return
	}
	createResp, err := client.Write(r.client.API.PostFunctionWithBodyWithResponse(ctx, client.JSONContentType, body))
	var created *function
	if err == nil {
		var createdBody *openapi.Function
		createdBody, err = client.Created(createResp.HTTPResponse, createResp.JSON200, "function")
		if err == nil {
			created, err = client.DecodeRaw(createdBody, createResp.Body)
		}
	}
	if err != nil {
		resp.Diagnostics.AddError("Error Creating Braintrust Function", err.Error())
		return
	}
	function, err := getFunction(ctx, r.client, created.Value.Id.String())
	if err != nil {
		mapFunctionToModel(created, &plan)
		resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
		resp.Diagnostics.AddError("Error Reading Braintrust Function After Create", fmt.Sprintf("Function was created (ID: %s) but could not be read: %s", created.Value.Id, err))
		return
	}
	mapFunctionToModel(function, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *functionResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state functionModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() || !requireClient(r.client, &resp.Diagnostics) {
		return
	}
	function, err := getFunction(ctx, r.client, state.ID.ValueString())
	if err != nil {
		if client.IsNotFoundError(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error Reading Braintrust Function", err.Error())
		return
	}
	mapFunctionToModel(function, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *functionResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state functionModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() || !requireClient(r.client, &resp.Diagnostics) {
		return
	}
	functionData, err := functionJSONObject(plan.FunctionDataJSON, "function_data_json")
	if err != nil {
		resp.Diagnostics.AddError("Invalid Function Data", err.Error())
		return
	}
	promptData, err := functionJSONObject(plan.PromptDataJSON, "prompt_data_json")
	if err != nil {
		resp.Diagnostics.AddError("Invalid Prompt Data", err.Error())
		return
	}
	update := openapi.PatchFunction{
		Name:        nullable.NewNullableWithValue(plan.Name.ValueString()),
		Description: patchString(plan.Description, state.Description),
	}
	if !plan.Tags.IsNull() && !plan.Tags.IsUnknown() {
		tags := stringListFromModel(plan.Tags)
		if tags == nil {
			tags = []string{}
		}
		update.Tags = nullable.NewNullableWithValue(tags)
	}
	raw := map[string]json.RawMessage{"function_data": functionData}
	if promptData != nil {
		raw["prompt_data"] = promptData
	} else if plan.PromptDataJSON.IsNull() && !state.PromptDataJSON.IsNull() {
		raw["prompt_data"] = json.RawMessage("null")
	}
	body, err := client.JSONBody(update, raw)
	if err != nil {
		resp.Diagnostics.AddError("Error Updating Braintrust Function", err.Error())
		return
	}
	updateResp, err := client.Write(r.client.API.PatchFunctionIdWithBodyWithResponse(ctx, state.ID.ValueString(), client.JSONContentType, body))
	var updated *function
	if err == nil {
		updated, err = client.DecodeRaw(updateResp.JSON200, updateResp.Body)
	}
	if err != nil {
		resp.Diagnostics.AddError("Error Updating Braintrust Function", err.Error())
		return
	}
	mapFunctionToModel(updated, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *functionResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state functionModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() || !requireClient(r.client, &resp.Diagnostics) {
		return
	}
	if _, err := client.Write(r.client.API.DeleteFunctionIdWithResponse(ctx, state.ID.ValueString())); err != nil && !client.IsNotFoundError(err) {
		resp.Diagnostics.AddError("Error Deleting Braintrust Function", err.Error())
	}
}

func (r *functionResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
