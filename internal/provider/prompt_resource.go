package provider

import (
	"context"
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

var (
	_ resource.Resource                = &promptResource{}
	_ resource.ResourceWithConfigure   = &promptResource{}
	_ resource.ResourceWithImportState = &promptResource{}
)

// NewPromptResource returns a new prompt resource.
func NewPromptResource() resource.Resource {
	return &promptResource{}
}

type promptResource struct {
	client *client.Client
}

func (r *promptResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_prompt"
}

func (r *promptResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Version:             0,
		Description:         "Manages a Braintrust prompt.",
		MarkdownDescription: "Manages a Braintrust prompt. This resource covers prompt object metadata and identity fields.",
		Attributes: map[string]schema.Attribute{
			"id":      computedStringAttribute("Unique identifier for the prompt (UUID)."),
			"xact_id": schema.StringAttribute{Computed: true, Description: "Transaction id of the prompt version."},
			"org_id":  computedStringAttribute("Unique identifier for the organization."),
			"created": computedStringAttribute("Date of prompt creation."),
			"project_id": schema.StringAttribute{
				Required:    true,
				Description: "Unique identifier for the project that the prompt belongs to.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"name": schema.StringAttribute{
				Required:    true,
				Description: "Name of the prompt.",
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"slug": schema.StringAttribute{
				Required:    true,
				Description: "Slug/identifier of the prompt. Must be unique within the project.",
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"description": optionalComputedStringAttribute("Textual description of the prompt."),
			"tags": schema.ListAttribute{
				Optional:    true,
				Computed:    true,
				ElementType: types.StringType,
				Description: "A list of tags for the prompt.",
			},
			"function_type": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Optional function type for the prompt.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"prompt_data_json": optionalComputedStringAttribute("JSON-encoded prompt_data payload."),
			"metadata_json": schema.StringAttribute{
				Computed:    true,
				Description: "JSON-encoded metadata object returned by the API.",
			},
		},
	}
}

func (r *promptResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = clientFromProviderData(req.ProviderData, &resp.Diagnostics)
}

func (r *promptResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan promptModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !requireClient(r.client, &resp.Diagnostics) {
		return
	}

	createReq := client.CreatePromptRequest{
		ProjectID:    plan.ProjectID.ValueString(),
		Name:         plan.Name.ValueString(),
		Slug:         plan.Slug.ValueString(),
		Description:  tfStringToPtr(plan.Description),
		Tags:         stringListFromModel(plan.Tags),
		FunctionType: tfStringToPtr(plan.FunctionType),
	}
	promptData, err := tfJSONStringToAny(plan.PromptDataJSON)
	if err != nil {
		resp.Diagnostics.AddError(
			"Invalid prompt_data_json",
			fmt.Sprintf("prompt_data_json must be valid JSON: %s", err.Error()),
		)
		return
	}
	createReq.PromptData = promptData

	tflog.Debug(ctx, "Creating prompt", map[string]any{
		"project_id": createReq.ProjectID,
		"name":       createReq.Name,
		"slug":       createReq.Slug,
	})

	created, err := r.client.CreatePrompt(ctx, createReq)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Creating Braintrust Prompt",
			"Could not create prompt, unexpected error: "+err.Error(),
		)
		return
	}

	prompt, err := r.client.GetPrompt(ctx, created.ID)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Reading Braintrust Prompt After Create",
			"Prompt was created (ID: "+created.ID+") but could not be read: "+err.Error(),
		)
		return
	}

	mapPromptToModel(prompt, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *promptResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state promptModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !requireClient(r.client, &resp.Diagnostics) {
		return
	}

	prompt, err := r.client.GetPrompt(ctx, state.ID.ValueString())
	if err != nil {
		if client.IsNotFoundError(err) {
			tflog.Warn(ctx, "Prompt not found, removing from state", map[string]any{
				"id": state.ID.ValueString(),
			})
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Error Reading Braintrust Prompt",
			"Could not read prompt ID "+state.ID.ValueString()+": "+err.Error(),
		)
		return
	}

	mapPromptToModel(prompt, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *promptResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan promptModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var state promptModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !requireClient(r.client, &resp.Diagnostics) {
		return
	}

	name := plan.Name.ValueString()
	slug := plan.Slug.ValueString()
	updateReq := client.UpdatePromptRequest{
		Name: &name,
		Slug: &slug,
	}

	if !plan.Description.IsNull() && !plan.Description.IsUnknown() {
		desc := plan.Description.ValueString()
		updateReq.Description = &desc
	} else if plan.Description.IsNull() && !state.Description.IsNull() {
		empty := ""
		updateReq.Description = &empty
	}
	if !plan.Tags.IsNull() && !plan.Tags.IsUnknown() {
		tags := stringListFromModel(plan.Tags)
		if tags == nil {
			tags = []string{}
		}
		updateReq.Tags = &tags
	}
	promptData, err := tfJSONStringToAny(plan.PromptDataJSON)
	if err != nil {
		resp.Diagnostics.AddError(
			"Invalid prompt_data_json",
			fmt.Sprintf("prompt_data_json must be valid JSON: %s", err.Error()),
		)
		return
	}
	updateReq.PromptData = promptData

	prompt, err := r.client.UpdatePrompt(ctx, state.ID.ValueString(), updateReq)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Updating Braintrust Prompt",
			"Could not update prompt ID "+state.ID.ValueString()+": "+err.Error(),
		)
		return
	}

	mapPromptToModel(prompt, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *promptResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state promptModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !requireClient(r.client, &resp.Diagnostics) {
		return
	}

	err := r.client.DeletePrompt(ctx, state.ID.ValueString())
	if err != nil {
		if client.IsNotFoundError(err) {
			return
		}
		resp.Diagnostics.AddError(
			"Error Deleting Braintrust Prompt",
			"Could not delete prompt ID "+state.ID.ValueString()+": "+err.Error(),
		)
	}
}

func (r *promptResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
