package provider

import (
	"context"

	"github.com/braintrustdata/braintrust-sdk-go/openapi"
	"github.com/braintrustdata/terraform-provider-braintrust/internal/client"
	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/oapi-codegen/nullable"
)

var (
	_ resource.Resource                 = &aiSecretResource{}
	_ resource.ResourceWithConfigure    = &aiSecretResource{}
	_ resource.ResourceWithImportState  = &aiSecretResource{}
	_ resource.ResourceWithUpgradeState = &aiSecretResource{}
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
		Version:     1,
		Description: "Manages a Braintrust AI provider secret.",
		MarkdownDescription: "Manages a Braintrust **organization-level** AI provider secret " +
			"(e.g. OpenAI, Anthropic, Azure API keys). These are visible in the Braintrust UI " +
			"under Organization > \"AI providers\" and are available to all projects.\n\n" +
			"For **project-level** AI provider credentials (visible under Project Settings > " +
			"\"Project AI providers\"), use `braintrust_env_var` with " +
			"`secret_type` set to the provider name instead.\n\n" +
			"`secret` is write-only and is never stored in the " +
			"resource's plan or state. Pass it through a variable with both `ephemeral = true` and " +
			"`sensitive = true` to keep the input out of saved plans as well. `secret_version` must " +
			"be a non-ephemeral positive integer and is stored in state.\n\n" +
			"Set both `secret` and `secret_version` when supplying a key. Change the version to rotate it; " +
			"changing only the secret does nothing. Metadata-only updates and removing the version do not " +
			"resend or clear the remote key. The API cannot clear AI secrets, and Terraform cannot detect " +
			"changes to their values made outside Terraform.\n\n" +
			"After import, omit both fields to manage metadata without rotation. Configuring a version " +
			"then writes the supplied secret. Upgrading older state discards its stored secret and starts " +
			"with no version, without changing the remote key. Historical state snapshots and backups " +
			"may still contain plaintext and must be protected. A masked `preview_secret` is available.\n\n" +
			"Creating a secret whose name already exists in Braintrust **overwrites** it. " +
			"Import existing secrets before managing them.",
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
				WriteOnly:   true,
				Description: "Write-only AI provider key. Requires secret_version. Use an ephemeral, sensitive input variable; changing this value alone does not rotate the key.",
				Validators: []validator.String{
					stringvalidator.AlsoRequires(path.MatchRoot("secret_version")),
				},
			},
			"secret_version": schema.Int64Attribute{
				Optional:    true,
				Description: "Non-sensitive, non-ephemeral positive version stored in state. Requires secret. Change this version to write the configured secret; omit both fields after import to leave the key untouched.",
				Validators: []validator.Int64{
					int64validator.AtLeast(1),
					int64validator.AlsoRequires(path.MatchRoot("secret")),
				},
			},
			"type": schema.StringAttribute{
				Optional:    true,
				Description: "The AI provider type (e.g. \"openai\", \"anthropic\", \"azure\", \"google\").",
			},
		},
	}
}

func (r *aiSecretResource) UpgradeState(context.Context) map[int64]resource.StateUpgrader {
	return map[int64]resource.StateUpgrader{0: aiSecretWriteOnly.stateUpgrader()}
}

func (r *aiSecretResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = clientFromProviderData(req.ProviderData, &resp.Diagnostics)
}

func (r *aiSecretResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan aiSecretModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	var config aiSecretModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	aiSecretWriteOnly.validate(config.Secret, plan.SecretVersion, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	if !requireClient(r.client, &resp.Diagnostics) {
		return
	}

	// Use PUT (replace) so that if an AI secret with this name already exists,
	// we update it rather than silently returning the old one.
	createReq := openapi.CreateAISecret{
		Name:   plan.Name.ValueString(),
		Secret: tfStringToNullable(config.Secret),
		Type:   tfStringToNullable(plan.Type),
	}

	tflog.Debug(ctx, "Creating AI secret", map[string]any{
		"name": createReq.Name,
		"type": plan.Type.ValueString(),
	})

	replaceResp, err := client.Write(r.client.API.PutAiSecretWithResponse(ctx, createReq))
	var created *openapi.AISecret
	if err == nil {
		created, err = client.Written(replaceResp.JSON200)
	}
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Creating Braintrust AI Secret",
			"Could not create AI secret, unexpected error: "+err.Error(),
		)
		return
	}

	// Re-read to get the full state.
	secret, err := getAISecret(ctx, r.client, created.Id.String())
	if err != nil {
		mapAISecretToModel(created, &plan)
		resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
		resp.Diagnostics.AddError(
			"Error Reading Braintrust AI Secret After Create",
			"AI secret was created (ID: "+created.Id.String()+") but could not be read: "+err.Error(),
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
	var config aiSecretModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	aiSecretWriteOnly.validate(config.Secret, plan.SecretVersion, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	if !requireClient(r.client, &resp.Diagnostics) {
		return
	}

	updateReq := openapi.PatchAISecret{
		// Always send name (it may have changed).
		Name: nullable.NewNullableWithValue(plan.Name.ValueString()),
		// Type is sent if set and cleared if removed.
		Type: patchString(plan.Type, state.Type),
	}
	if aiSecretWriteOnly.versionChanged(plan.SecretVersion, state.SecretVersion) {
		updateReq.Secret = tfStringToNullable(config.Secret)
	}

	tflog.Debug(ctx, "Updating AI secret", map[string]any{"id": state.ID.ValueString()})

	updateResp, err := client.Write(r.client.API.PatchAiSecretIdWithResponse(ctx, state.ID.ValueString(), updateReq))
	var secret *openapi.AISecret
	if err == nil {
		secret, err = client.Written(updateResp.JSON200)
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
