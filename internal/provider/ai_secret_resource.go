package provider

import (
	"context"

	"github.com/braintrustdata/terraform-provider-braintrust/internal/client"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-log/tflog"
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
			"\"Project AI providers\"), use [`braintrust_env_var`](env_var) with " +
			"`secret_type` set to the provider name instead.\n\n" +
			"The `secret` attribute is write-only. Terraform sends it during create/update but the API " +
			"never returns it. A masked `preview_secret` is available for verification.\n\n" +
			"**Tip:** Use Terraform variables with `sensitive = true` for the secret value:\n\n" +
			"```hcl\nvariable \"openai_api_key\" {\n  type      = string\n  sensitive = true\n}\n\n" +
			"resource \"braintrust_ai_secret\" \"openai\" {\n  name   = \"OpenAI\"\n  type   = \"openai\"\n  secret = var.openai_api_key\n}\n```",
		Attributes: map[string]schema.Attribute{
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
				Description: "The secret value (e.g. API key). Write-only: the API never returns this value.",
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

	// Use PUT (replace) so that if an AI secret with this name already exists,
	// we update it rather than silently returning the old one.
	createReq := client.CreateAISecretRequest{
		Name:   plan.Name.ValueString(),
		Secret: tfStringToPtr(plan.Secret),
		Type:   tfStringToPtr(plan.Type),
	}

	tflog.Debug(ctx, "Creating AI secret", map[string]any{
		"name": createReq.Name,
		"type": createReq.Type,
	})

	created, err := r.client.ReplaceAISecret(ctx, createReq)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Creating Braintrust AI Secret",
			"Could not create AI secret, unexpected error: "+err.Error(),
		)
		return
	}

	// Re-read to get the full state.
	secret, err := r.client.GetAISecret(ctx, created.ID)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Reading Braintrust AI Secret After Create",
			"AI secret was created (ID: "+created.ID+") but could not be read: "+err.Error(),
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

	secret, err := r.client.GetAISecret(ctx, state.ID.ValueString())
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

	updateReq := client.UpdateAISecretRequest{}

	// Always send name (it may have changed).
	name := plan.Name.ValueString()
	updateReq.Name = &name

	// Handle secret: send if set, but the API doesn't support clearing secrets
	// (no "set to empty" semantic), so we only send if non-null.
	if !plan.Secret.IsNull() && !plan.Secret.IsUnknown() {
		s := plan.Secret.ValueString()
		updateReq.Secret = &s
	}

	// Handle type: send if set, clear if removed.
	if !plan.Type.IsNull() && !plan.Type.IsUnknown() {
		t := plan.Type.ValueString()
		updateReq.Type = &t
	} else if plan.Type.IsNull() && !state.Type.IsNull() {
		empty := ""
		updateReq.Type = &empty
	}

	tflog.Debug(ctx, "Updating AI secret", map[string]any{"id": state.ID.ValueString()})

	secret, err := r.client.UpdateAISecret(ctx, state.ID.ValueString(), updateReq)
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

	err := r.client.DeleteAISecret(ctx, state.ID.ValueString())
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
