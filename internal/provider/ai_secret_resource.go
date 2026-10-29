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
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/oapi-codegen/nullable"
)

var (
	_ resource.Resource                = &aiSecretResource{}
	_ resource.ResourceWithConfigure   = &aiSecretResource{}
	_ resource.ResourceWithImportState = &aiSecretResource{}
)

// NewAISecretResource returns a new AI secret resource.
func NewAISecretResource() resource.Resource {
	return &aiSecretResource{}
}

type aiSecretResource struct {
	client *client.Client
}

func (r *aiSecretResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ai_secret"
}

func (r *aiSecretResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Version:     0,
		Description: "Manages a Braintrust AI provider secret.",
		MarkdownDescription: "Manages a Braintrust **organization-level** AI provider secret " +
			"(e.g. OpenAI, Anthropic, Azure API keys). These are visible in the Braintrust UI " +
			"under Organization > \"AI providers\" and are available to all projects.\n\n" +
			"For **project-level** AI provider credentials (visible under Project Settings > " +
			"\"Project AI providers\"), use `braintrust_env_var` with " +
			"`secret_type` set to the provider name instead.\n\n" +
			"The API never returns `secret`, so Terraform keeps the configured value in state and cannot " +
			"detect changes made outside Terraform. A masked `preview_secret` is available for verification. " +
			"Pass the secret through a variable with `sensitive = true`.\n\n" +
			"Creating a secret whose name already exists in Braintrust **overwrites** it. " +
			"Import existing secrets before managing them.",
		Attributes: map[string]schema.Attribute{
			"metadata":   schema.StringAttribute{Optional: true, Description: "Provider-specific metadata as a JSON object; use jsonencode({...}). Braintrust recursively merges this field on PATCH. Removing metadata or existing object keys is unsupported; replace the resource to clear them."},
			"id":         computedStringAttribute("Unique identifier for the AI secret (UUID)."),
			"org_id":     computedStringAttribute("Unique identifier for the organization."),
			"created":    computedStringAttribute("Date of AI secret creation."),
			"updated_at": schema.StringAttribute{Computed: true, Description: "Date of last AI secret update."},
			"preview_secret": schema.StringAttribute{
				Computed:    true,
				Description: "A masked preview of the secret value.",
			},
			"name": schema.StringAttribute{
				Required:    true,
				Description: "Name of the AI secret. Must be unique within the organization.",
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"secret": schema.StringAttribute{
				Optional:    true,
				Sensitive:   true,
				Description: "The secret value (e.g. API key). The API never returns this value; Terraform stores the configured value in state.",
			},
			"type": schema.StringAttribute{
				Optional:    true,
				Description: "The AI provider type (e.g. \"openai\", \"anthropic\", \"azure\", \"google\").",
			},
		},
	}
}

func (r *aiSecretResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = clientFromProviderData(req.ProviderData, &resp.Diagnostics)
}

func (r *aiSecretResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan aiSecretModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !requireClient(r.client, &resp.Diagnostics) {
		return
	}

	metadata, err := functionJSONObject(plan.Metadata, "metadata")
	if err != nil {
		resp.Diagnostics.AddAttributeError(path.Root("metadata"), "Invalid Metadata", err.Error())
		return
	}

	// Use PUT (replace) so that if an AI secret with this name already exists,
	// we update it rather than silently returning the old one.
	createReq := openapi.CreateAISecret{
		Name:   plan.Name.ValueString(),
		Secret: tfStringToNullable(plan.Secret),
		Type:   tfStringToNullable(plan.Type),
	}

	tflog.Debug(ctx, "Creating AI secret", map[string]any{
		"name": createReq.Name,
		"type": plan.Type.ValueString(),
	})

	body, err := client.JSONBody(createReq, map[string]json.RawMessage{"metadata": metadata})
	if err != nil {
		resp.Diagnostics.AddError("Error Creating Braintrust AI Secret", err.Error())
		return
	}
	replaceResp, err := client.Write(r.client.API.PutAiSecretWithBodyWithResponse(ctx, client.JSONContentType, body))
	var created *aiSecret
	if err == nil {
		created, err = client.WrittenRaw(replaceResp.JSON200, replaceResp.Body)
	}
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Creating Braintrust AI Secret",
			"Could not create AI secret, unexpected error: "+err.Error(),
		)
		return
	}

	// Re-read to get the full state.
	secret, err := getAISecret(ctx, r.client, created.Value.Id.String())
	if err != nil {
		mapAISecretToModel(created, &plan)
		resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
		resp.Diagnostics.AddError(
			"Error Reading Braintrust AI Secret After Create",
			"AI secret was created (ID: "+created.Value.Id.String()+") but could not be read: "+err.Error(),
		)
		return
	}

	mapAISecretToModel(secret, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *aiSecretResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state aiSecretModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !requireClient(r.client, &resp.Diagnostics) {
		return
	}

	secret, err := getAISecret(ctx, r.client, state.ID.ValueString())
	if err != nil {
		if client.IsNotFoundError(err) {
			tflog.Warn(ctx, "AI secret not found, removing from state", map[string]any{
				"id": state.ID.ValueString(),
			})
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Error Reading Braintrust AI Secret",
			"Could not read AI secret ID "+state.ID.ValueString()+": "+err.Error(),
		)
		return
	}

	// mapAISecretToModel preserves the existing state.Secret (write-only).
	mapAISecretToModel(secret, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *aiSecretResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan aiSecretModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var state aiSecretModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !requireClient(r.client, &resp.Diagnostics) {
		return
	}

	metadata, err := functionJSONObject(plan.Metadata, "metadata")
	if err != nil {
		resp.Diagnostics.AddAttributeError(path.Root("metadata"), "Invalid Metadata", err.Error())
		return
	}
	if !state.Metadata.IsNull() && (metadata == nil || viewJSONRemovesKeys(state.Metadata.ValueString(), string(metadata))) {
		resp.Diagnostics.AddAttributeError(path.Root("metadata"), "Cannot Remove AI Secret Metadata", "Braintrust recursively merges AI secret metadata on PATCH and ignores null. Removing metadata or existing object keys cannot clear them on the server. Keep existing keys or explicitly replace the resource to clear metadata.")
		return
	}

	if scoreJSONEqual(plan.Metadata, state.Metadata) {
		metadata = nil
	}

	updateReq := openapi.PatchAISecret{
		// Always send name (it may have changed).
		Name: nullable.NewNullableWithValue(plan.Name.ValueString()),
		// The API doesn't support clearing secrets (no "set to empty"
		// semantic), so the secret is only sent if set.
		Secret: tfStringToNullable(plan.Secret),
		// Type is sent if set and cleared if removed.
		Type: patchString(plan.Type, state.Type),
	}

	tflog.Debug(ctx, "Updating AI secret", map[string]any{"id": state.ID.ValueString()})

	body, err := client.JSONBody(updateReq, map[string]json.RawMessage{"metadata": metadata})
	if err != nil {
		resp.Diagnostics.AddError("Error Updating Braintrust AI Secret", err.Error())
		return
	}
	updateResp, err := client.Write(r.client.API.PatchAiSecretIdWithBodyWithResponse(ctx, state.ID.ValueString(), client.JSONContentType, body))
	var secret *aiSecret
	if err == nil {
		secret, err = client.WrittenRaw(updateResp.JSON200, updateResp.Body)
	}
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Updating Braintrust AI Secret",
			"Could not update AI secret ID "+state.ID.ValueString()+": "+err.Error(),
		)
		return
	}

	mapAISecretToModel(secret, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *aiSecretResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state aiSecretModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !requireClient(r.client, &resp.Diagnostics) {
		return
	}

	_, err := client.Write(r.client.API.DeleteAiSecretIdWithResponse(ctx, state.ID.ValueString()))
	if err != nil {
		if client.IsNotFoundError(err) {
			return
		}
		resp.Diagnostics.AddError(
			"Error Deleting Braintrust AI Secret",
			"Could not delete AI secret ID "+state.ID.ValueString()+": "+err.Error(),
		)
	}
}

func (r *aiSecretResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
