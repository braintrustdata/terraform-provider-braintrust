package provider

import (
	"context"
	"errors"
	"time"

	"github.com/braintrustdata/terraform-provider-braintrust/internal/client"
	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource                = &apiKeyResource{}
	_ resource.ResourceWithConfigure   = &apiKeyResource{}
	_ resource.ResourceWithImportState = &apiKeyResource{}
)

// NewAPIKeyResource returns a managed user API key resource.
func NewAPIKeyResource() resource.Resource { return &apiKeyResource{} }

type apiKeyResource struct{ client *client.Client }

func (r *apiKeyResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_api_key"
}

func (r *apiKeyResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Braintrust user API key.",
		MarkdownDescription: "Manages a Braintrust user API key. Creation requires an unrestricted user credential and creates a key for that same user; service tokens, OAuth credentials, and restricted keys cannot mint user keys. " +
			"For an organization-scoped caller, omitting `org_id` uses the caller's organization and an explicit organization must match it. An unscoped user credential must supply `org_id` selecting an organization the user belongs to. The API caps the lifetime to the caller's remaining lifetime; `expires_at` reports the actual expiration.\n\n" +
			"**The raw `key` is returned only at creation and is stored in Terraform state. Protect state as sensitive.** Refresh preserves the stored key but cannot recover it. Import by ID manages metadata and revocation only: an imported key is null, not retrievable. Importing a key authorizes Terraform to revoke it on destroy or replacement.\n\n" +
			"Keys are immutable: changes to `name`, `org_id`, or `expires_in_seconds` require replacement. Use a unique name in the organization; existing matching keys are rejected, never adopted. When organization is omitted, any visible same-name key is conservatively rejected. " +
			"If creation is uncertain, the provider never retries the POST. Only an exact new response ID with verified metadata can be retained in tainted state for revocation on the next apply; otherwise inspect and revoke the key manually before retrying. Concurrent same-name keys are never adopted or revoked by recovery.",
		Attributes: map[string]schema.Attribute{
			"id": computedStringAttribute("Unique identifier for the API key (UUID)."),
			"name": schema.StringAttribute{
				Required: true, Description: "Name of the API key. Use a unique name within the organization. Changing it replaces the key.",
				Validators:    []validator.String{stringvalidator.LengthAtLeast(1)},
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"org_id": schema.StringAttribute{
				Optional: true, Computed: true, Description: "Organization ID. Defaults to a scoped caller's organization and cannot select a different organization. Required for an unscoped user credential, selecting a member organization. Changing it replaces the key.",
				Validators:    []validator.String{stringvalidator.LengthAtLeast(1)},
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace(), stringplanmodifier.UseStateForUnknown()},
			},
			"expires_in_seconds": schema.Int64Attribute{
				Optional: true, Description: "Requested positive lifetime in seconds. The API caps it to the caller's remaining lifetime. Omit to use the API default. Never inferred from expires_at during refresh or import; changing it replaces the key.",
				Validators:    []validator.Int64{int64validator.AtLeast(1)},
				PlanModifiers: []planmodifier.Int64{int64planmodifier.RequiresReplace()},
			},
			"key": schema.StringAttribute{
				Computed: true, Sensitive: true, Description: "Raw API key, returned only at creation and preserved in sensitive Terraform state. Null after import; cannot be retrieved from the API.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"created":          computedStringAttribute("Date of API key creation."),
			"preview_name":     computedStringAttribute("Masked preview of the API key."),
			"user_id":          computedStringAttribute("ID of the user who owns the API key."),
			"user_email":       computedStringAttribute("Email of the user who owns the API key."),
			"user_given_name":  computedStringAttribute("Given name of the user who owns the API key."),
			"user_family_name": computedStringAttribute("Family name of the user who owns the API key."),
			"expires_at":       computedStringAttribute("Actual expiration timestamp, including any lifetime cap imposed by the API. Null when the key does not expire."),
		},
	}
}

func (r *apiKeyResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = clientFromProviderData(req.ProviderData, &resp.Diagnostics)
}

func (r *apiKeyResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan apiKeyModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() || !requireClient(r.client, &resp.Diagnostics) {
		return
	}
	input := client.CreateAPIKeyRequest{Name: plan.Name.ValueString()}
	if plan.Name.IsUnknown() || input.Name == "" || plan.ExpiresInSeconds.IsUnknown() {
		resp.Diagnostics.AddError("Invalid API Key Configuration", "name must be known and nonempty, and expires_in_seconds must be known or omitted before creation.")
		return
	}
	if !plan.OrgID.IsNull() && !plan.OrgID.IsUnknown() {
		orgID, err := uuid.Parse(plan.OrgID.ValueString())
		if err != nil || orgID == uuid.Nil {
			resp.Diagnostics.AddAttributeError(path.Root("org_id"), "Invalid Organization ID", "org_id must be a nonzero UUID.")
			return
		}
		org := plan.OrgID.ValueString()
		input.OrgID = &org
	}
	if !plan.ExpiresInSeconds.IsNull() {
		seconds := plan.ExpiresInSeconds.ValueInt64()
		if seconds <= 0 {
			resp.Diagnostics.AddAttributeError(path.Root("expires_in_seconds"), "Invalid API Key Lifetime", "expires_in_seconds must be greater than zero.")
			return
		}
		// Keep int64 end-to-end; do not convert to int or time.Duration.
		input.ExpiresInSeconds = &seconds
	}
	existing, err := listAPIKeysByName(ctx, r.client, input.Name)
	if err != nil {
		resp.Diagnostics.AddError("Error Checking Existing API Keys", "No key was created. "+apiKeyError(err))
		return
	}
	knownIDs := make(map[string]bool, len(existing))
	for _, key := range existing {
		knownIDs[key.Id.String()] = true
		if apiKeyMatches(key, input.Name, input.OrgID) {
			resp.Diagnostics.AddError("API Key Name Already Used", "An API key with this name already exists; import ID "+key.Id.String()+" to manage metadata and revocation only, or use a unique name. It was not adopted or deleted, and its raw key cannot be recovered.")
			return
		}
	}
	output, err := r.client.CreateAPIKey(ctx, input)
	if err != nil {
		if client.IsUncertainWriteError(err) || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			r.recoverUncertainCreate(ctx, input, output, knownIDs, &plan, resp)
		} else {
			resp.Diagnostics.AddError("Error Creating Braintrust API Key", apiKeyError(err))
		}
		return
	}
	if knownIDs[output.Id.String()] {
		resp.Diagnostics.AddError("API Key Was Not Newly Created", "The API returned a preexisting key ID: "+output.Id.String()+". It was not adopted or deleted. Inspect the remote keys before retrying.")
		return
	}
	if !apiKeyMatches(output.ApiKey, input.Name, input.OrgID) {
		resp.Diagnostics.AddError("API Key Response Did Not Match", "The create response metadata did not match the requested name and organization. No key was adopted or deleted. Inspect and revoke the newly created key (ID: "+output.Id.String()+") before retrying; its raw key cannot be recovered.")
		return
	}
	mapAPIKeyToModel(&output.ApiKey, &plan)
	plan.Key = types.StringValue(output.Key)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *apiKeyResource) recoverUncertainCreate(ctx context.Context, input client.CreateAPIKeyRequest, output *client.CreateAPIKeyResponse, knownIDs map[string]bool, plan *apiKeyModel, resp *resource.CreateResponse) {
	detail := "The POST was not retried. No key was adopted or deleted. Inspect keys with the configured name and organization and revoke only a key created by this attempt before retrying; its raw key cannot be recovered."
	if output != nil && output.Id != uuid.Nil {
		id := output.Id.String()
		detail += " The create response identified ID: " + id + "."
		if !knownIDs[id] && apiKeyMatches(output.ApiKey, input.Name, input.OrgID) {
			// Cancellation of POST must not prevent a bounded, read-only recovery.
			recoveryCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
			defer cancel()
			key, err := getAPIKey(recoveryCtx, r.client, id)
			if err == nil && key.Id == output.Id && apiKeyMatches(*key, input.Name, input.OrgID) &&
				nullableUUIDToTF(key.OrgId).Equal(nullableUUIDToTF(output.OrgId)) &&
				nullableUUIDToTF(key.UserId).Equal(nullableUUIDToTF(output.UserId)) &&
				!nullableUUIDToTF(key.OrgId).IsNull() && !nullableUUIDToTF(key.UserId).IsNull() {
				mapAPIKeyToModel(key, plan)
				plan.Key = types.StringNull()
				resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
				resp.Diagnostics.AddError("API Key Created But Key Was Lost", "The exact new create-response ID ("+id+") was verified by name, organization, and owner. Terraform retained it in tainted state and will revoke it before recreating on the next apply. The POST was not retried and the raw key cannot be recovered.")
				return
			}
		}
	}
	resp.Diagnostics.AddError("API Key Creation Outcome Uncertain", detail)
}

func (r *apiKeyResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state apiKeyModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() || !requireClient(r.client, &resp.Diagnostics) {
		return
	}
	key, err := getAPIKey(ctx, r.client, state.ID.ValueString())
	if err != nil {
		if client.IsNotFoundError(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error Reading Braintrust API Key", apiKeyError(err))
		return
	}
	mapAPIKeyToModel(key, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *apiKeyResource) Update(_ context.Context, _ resource.UpdateRequest, resp *resource.UpdateResponse) {
	resp.Diagnostics.AddError("API Key Update Not Supported", "API keys are immutable. This is a provider bug: changing configuration must trigger replacement.")
}

func (r *apiKeyResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state apiKeyModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() || !requireClient(r.client, &resp.Diagnostics) {
		return
	}
	_, err := client.Write(r.client.API.DeleteApiKeyIdWithResponse(ctx, state.ID.ValueString()))
	if err != nil && !client.IsNotFoundError(err) {
		resp.Diagnostics.AddError("Error Deleting Braintrust API Key", "Could not confirm revocation. "+apiKeyError(err))
	}
}

func (r *apiKeyResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	id, err := uuid.Parse(req.ID)
	if err != nil || id == uuid.Nil {
		resp.Diagnostics.AddError("Invalid API Key Import ID", "Import requires a nonzero API key UUID.")
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, apiKeyModel{ID: types.StringValue(id.String()), Key: types.StringNull(), ExpiresInSeconds: types.Int64Null()})...)
}
