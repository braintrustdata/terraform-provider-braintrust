package provider

import (
	"context"

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
)

var (
	_ resource.Resource                = &envVarResource{}
	_ resource.ResourceWithConfigure   = &envVarResource{}
	_ resource.ResourceWithImportState = &envVarResource{}
)

// NewEnvVarResource returns a new env var resource.
func NewEnvVarResource() resource.Resource {
	return &envVarResource{}
}

type envVarResource struct {
	client *client.Client
}

func (r *envVarResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_env_var"
}

func (r *envVarResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Version:     0,
		Description: "Manages a Braintrust environment variable.",
		MarkdownDescription: "Manages a Braintrust environment variable. Environment variables can be scoped " +
			"to an organization, project, or function. Values are encrypted at rest.\n\n" +
			"This resource also manages **project-level AI provider credentials**. To configure a " +
			"project-scoped AI provider key (visible in the Braintrust UI under Project Settings > " +
			"\"Project AI providers\"), set `secret_type` to the provider name (e.g. `\"openai\"`, " +
			"`\"anthropic\"`). The API will automatically categorize it as an AI provider.\n\n" +
			"For **organization-level** AI provider credentials (visible under Organization > " +
			"\"AI providers\"), use [`braintrust_ai_secret`](ai_secret) instead.\n\n" +
			"**Note:** The `value` attribute is write-only. Terraform stores it in state but the API " +
			"never returns it, so it cannot be imported.",
		Attributes: map[string]schema.Attribute{
			"id": computedStringAttribute("Unique identifier for the environment variable (UUID)."),
			"object_type": schema.StringAttribute{
				Required:    true,
				Description: "The type of the object the environment variable is scoped for.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					stringvalidator.OneOf("organization", "project", "function"),
				},
			},
			"object_id": schema.StringAttribute{
				Required:    true,
				Description: "The ID of the object the environment variable is scoped for.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"name": schema.StringAttribute{
				Required:    true,
				Description: "The name of the environment variable.",
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"value": schema.StringAttribute{
				Optional:    true,
				Sensitive:   true,
				Description: "The value of the environment variable. Encrypted at rest. Write-only: the API never returns this value.",
			},
			"created":     computedStringAttribute("Date of environment variable creation."),
			"used":        computedStringAttribute("Date the environment variable was last used."),
			"secret_type": optionalComputedStringAttribute("The AI provider name (e.g. \"openai\", \"anthropic\"). When set, the env var appears under \"Project AI providers\" in the Braintrust UI."),
			"secret_category": schema.StringAttribute{
				Computed:    true,
				Description: "The category of the secret: env_var for regular environment variables, ai_provider for AI provider API keys.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

func (r *envVarResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = clientFromProviderData(req.ProviderData, &resp.Diagnostics)
}

func (r *envVarResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan envVarModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !requireClient(r.client, &resp.Diagnostics) {
		return
	}

	// Use PUT (replace) so that if an env var with this name already exists
	// in the same scope, we update it rather than silently returning the old one.
	objectID, err := parseUUID(plan.ObjectID, "object_id")
	if err != nil {
		resp.Diagnostics.AddAttributeError(path.Root("object_id"), "Invalid Object ID", err.Error())
		return
	}
	replaceReq := openapi.PutEnvVarJSONRequestBody{
		ObjectType: openapi.PutEnvVarJSONBodyObjectType(plan.ObjectType.ValueString()),
		ObjectId:   objectID,
		Name:       plan.Name.ValueString(),
		Value:      tfStringToNullable(plan.Value),
		SecretType: tfStringToNullable(plan.SecretType),
	}

	tflog.Debug(ctx, "Creating env var", map[string]any{
		"object_type": replaceReq.ObjectType,
		"object_id":   plan.ObjectID.ValueString(),
		"name":        replaceReq.Name,
	})

	replaceResp, err := client.Write(r.client.API.PutEnvVarWithResponse(ctx, replaceReq))
	var created *openapi.EnvVar
	if err == nil {
		created, err = client.Written(replaceResp.JSON200)
	}
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Creating Braintrust Env Var",
			"Could not create env var, unexpected error: "+err.Error(),
		)
		return
	}

	// Re-read to get full state.
	envVar, err := getEnvVar(ctx, r.client, created.Id.String())
	if err != nil {
		mapEnvVarToModel(created, &plan)
		resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
		resp.Diagnostics.AddError(
			"Error Reading Braintrust Env Var After Create",
			"Env var was created (ID: "+created.Id.String()+") but could not be read: "+err.Error(),
		)
		return
	}

	mapEnvVarToModel(envVar, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *envVarResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state envVarModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !requireClient(r.client, &resp.Diagnostics) {
		return
	}

	envVar, err := getEnvVar(ctx, r.client, state.ID.ValueString())
	if err != nil {
		if client.IsNotFoundError(err) {
			tflog.Warn(ctx, "Env var not found, removing from state", map[string]any{
				"id": state.ID.ValueString(),
			})
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Error Reading Braintrust Env Var",
			"Could not read env var ID "+state.ID.ValueString()+": "+err.Error(),
		)
		return
	}

	mapEnvVarToModel(envVar, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *envVarResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan envVarModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var state envVarModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !requireClient(r.client, &resp.Diagnostics) {
		return
	}

	// Value and secret_type are sent when set and cleared when removed from config.
	updateReq := openapi.PatchEnvVarIdJSONRequestBody{
		Name:       plan.Name.ValueString(),
		Value:      patchString(plan.Value, state.Value),
		SecretType: patchString(plan.SecretType, state.SecretType),
	}

	tflog.Debug(ctx, "Updating env var", map[string]any{"id": state.ID.ValueString()})

	updateResp, err := client.Write(r.client.API.PatchEnvVarIdWithResponse(ctx, state.ID.ValueString(), updateReq))
	var envVar *openapi.EnvVar
	if err == nil {
		envVar, err = client.Written(updateResp.JSON200)
	}
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Updating Braintrust Env Var",
			"Could not update env var ID "+state.ID.ValueString()+": "+err.Error(),
		)
		return
	}

	mapEnvVarToModel(envVar, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *envVarResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state envVarModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !requireClient(r.client, &resp.Diagnostics) {
		return
	}

	_, err := client.Write(r.client.API.DeleteEnvVarIdWithResponse(ctx, state.ID.ValueString()))
	if err != nil {
		if client.IsNotFoundError(err) {
			return
		}
		resp.Diagnostics.AddError(
			"Error Deleting Braintrust Env Var",
			"Could not delete env var ID "+state.ID.ValueString()+": "+err.Error(),
		)
	}
}

func (r *envVarResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
