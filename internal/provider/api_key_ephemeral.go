package provider

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/braintrustdata/terraform-provider-braintrust/internal/client"
	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/ephemeral"
	"github.com/hashicorp/terraform-plugin-framework/ephemeral/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ ephemeral.EphemeralResource              = &apiKeyEphemeralResource{}
	_ ephemeral.EphemeralResourceWithConfigure = &apiKeyEphemeralResource{}
)

// NewAPIKeyEphemeralResource returns a state-free API key generator.
func NewAPIKeyEphemeralResource() ephemeral.EphemeralResource {
	return &apiKeyEphemeralResource{}
}

type apiKeyEphemeralResource struct {
	client *client.Client
}

type apiKeyEphemeralModel struct {
	ID               types.String `tfsdk:"id"`
	Name             types.String `tfsdk:"name"`
	OrgID            types.String `tfsdk:"org_id"`
	ExpiresInSeconds types.Int64  `tfsdk:"expires_in_seconds"`
	Nonce            types.String `tfsdk:"nonce"`
	Key              types.String `tfsdk:"key"`
}

func (r *apiKeyEphemeralResource) Metadata(_ context.Context, req ephemeral.MetadataRequest, resp *ephemeral.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_api_key"
}

func (r *apiKeyEphemeralResource) Schema(_ context.Context, _ ephemeral.SchemaRequest, resp *ephemeral.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Generates a Braintrust user API key without storing its data in Terraform plan or state files.",
		MarkdownDescription: "Generates a Braintrust user API key without storing its data in Terraform plan or state files.\n\n" +
			"**Ephemeral describes Terraform data, not the remote key lifetime.** The provider does not revoke the key on Close or configuration destruction, and does not automatically rotate it. Callers own secure delivery, store lookup, rotation, expiration, and revocation. The raw `key` is available only at creation.\n\n" +
			"**Set `nonce = timestamp()` (or `uuid()`) to defer minting until apply.** Terraform opens ephemeral resources during plan when all inputs are known; a literal nonce can mint a key on every plan. Each Open creates a new key. Existing visible same-name keys in the requested organization are rejected, never adopted. When `org_id` is omitted, this safety check conservatively rejects visible same-name keys in any organization.\n\n" +
			"Creation requires an unrestricted user API key: service tokens, OAuth credentials, and restricted keys cannot mint user keys. The new key belongs to the caller's user. For organization-scoped credentials, `org_id` defaults to and must match the caller's organization. Unscoped user credentials must supply `org_id`, selecting an organization the caller belongs to. A positive `expires_in_seconds` is capped by the caller's remaining lifetime.\n\n" +
			"An uncertain POST is never retried. Cleanup only revokes a sole newly observed matching key whose ID was returned by this create attempt. Ambiguous candidates require manual inspection and revocation. No recovery ID is retained in state. If downstream delivery fails after a successful Open, inspect and revoke the key before minting a replacement.",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				Required:    true,
				Description: "Name of the key. Use a unique name in the target organization for safe creation and recovery.",
				Validators:  []validator.String{stringvalidator.LengthAtLeast(1)},
			},
			"org_id": schema.StringAttribute{
				Optional:    true,
				Description: "Organization ID. For organization-scoped credentials, defaults to and must match the caller's organization. Required for unscoped user credentials and must select an organization the caller belongs to.",
				Validators:  []validator.String{stringvalidator.LengthAtLeast(1)},
			},
			"expires_in_seconds": schema.Int64Attribute{
				Optional:    true,
				Description: "Positive key lifetime in seconds, capped by the caller's remaining lifetime. Omit to use the API's default lifetime.",
				Validators:  []validator.Int64{int64validator.AtLeast(1)},
			},
			"nonce": schema.StringAttribute{
				Required:    true,
				Description: "Set to timestamp() or uuid() to defer minting until apply. Not sent to the API. A literal value can mint a key during plan.",
				Validators:  []validator.String{stringvalidator.LengthAtLeast(1)},
			},
			"id": schema.StringAttribute{
				Computed:    true,
				Sensitive:   true,
				Description: "ID of the created key. Deliver it securely with the key so it can later be revoked.",
			},
			"key": schema.StringAttribute{
				Computed:    true,
				Sensitive:   true,
				Description: "Raw key, available only at creation. Never stored in Terraform plan or state files; callers must deliver it securely.",
			},
		},
	}
}

func (r *apiKeyEphemeralResource) Configure(_ context.Context, req ephemeral.ConfigureRequest, resp *ephemeral.ConfigureResponse) {
	r.client = clientFromProviderData(req.ProviderData, &resp.Diagnostics)
}

func (r *apiKeyEphemeralResource) Open(ctx context.Context, req ephemeral.OpenRequest, resp *ephemeral.OpenResponse) {
	var config apiKeyEphemeralModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() || !requireClient(r.client, &resp.Diagnostics) {
		return
	}
	if config.Name.IsUnknown() || config.Name.IsNull() || config.Name.ValueString() == "" ||
		config.Nonce.IsUnknown() || config.Nonce.IsNull() || config.Nonce.ValueString() == "" ||
		config.OrgID.IsUnknown() || (!config.OrgID.IsNull() && config.OrgID.ValueString() == "") ||
		config.ExpiresInSeconds.IsUnknown() {
		resp.Diagnostics.AddError("Invalid API Key Configuration", "name and nonce must be known and nonempty; org_id must be known and nonempty or omitted, and expires_in_seconds must be known or omitted before opening the ephemeral resource.")
		return
	}
	createReq := client.CreateAPIKeyRequest{Name: config.Name.ValueString()}
	if !config.OrgID.IsNull() {
		orgID := config.OrgID.ValueString()
		createReq.OrgID = &orgID
	}
	if !config.ExpiresInSeconds.IsNull() {
		seconds := config.ExpiresInSeconds.ValueInt64()
		if seconds <= 0 {
			resp.Diagnostics.AddError("Invalid API Key Lifetime", "expires_in_seconds must be a positive integer.")
			return
		}
		createReq.ExpiresInSeconds = &seconds
	}

	existing, err := listAPIKeysByName(ctx, r.client, createReq.Name)
	if err != nil {
		resp.Diagnostics.AddError("Error Checking Existing API Keys", "No key was created. "+apiKeyError(err))
		return
	}
	knownIDs := make(map[string]bool, len(existing))
	for _, key := range existing {
		knownIDs[key.Id.String()] = true
		if apiKeyMatches(key, createReq.Name, createReq.OrgID) {
			resp.Diagnostics.AddError("API Key Name Already Used", "A visible key with this name already exists in the requested scope (ID: "+key.Id.String()+"). Its raw key cannot be recovered. Use a unique name or revoke it before creating another; it was not adopted or deleted.")
			return
		}
	}

	// A lost creation response cannot be recovered by repeating the POST.
	created, err := r.client.CreateAPIKey(ctx, createReq)
	responseID := ""
	if created != nil && created.Id != uuid.Nil {
		responseID = created.Id.String()
	}
	if err != nil {
		if client.IsUncertainWriteError(err) || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			// Cancellation of Open must not prevent bounded cleanup.
			cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), apiKeyCleanupTimeout)
			defer cancel()
			resp.Diagnostics.AddError("API Key Creation Outcome Uncertain", r.reconcileUncertainOpen(cleanupCtx, createReq, knownIDs, created)+" "+apiKeyError(err))
		} else {
			resp.Diagnostics.AddError("Error Creating Braintrust API Key", "Key creation failed. "+apiKeyError(err))
		}
		return
	}
	if knownIDs[responseID] {
		resp.Diagnostics.AddError("API Key Was Not Newly Created", "The API returned a preexisting key ID: "+responseID+". It was not adopted or deleted. Inspect the remote keys before retrying.")
		return
	}

	// Configured inputs are retained verbatim; outputs exist only for this Open.
	config.ID = types.StringValue(responseID)
	config.Key = types.StringValue(created.Key)
	resp.Diagnostics.Append(resp.Result.Set(ctx, config)...)
}

const apiKeyCleanupTimeout = 30 * time.Second

func (r *apiKeyEphemeralResource) reconcileUncertainOpen(ctx context.Context, req client.CreateAPIKeyRequest, knownIDs map[string]bool, created *client.CreateAPIKeyResponse) string {
	detail := "The POST was not retried, and no recovery ID is retained in Terraform state."
	responseID := ""
	if created != nil && created.Id != uuid.Nil {
		responseID = created.Id.String()
		detail += " The create response identified key ID: " + responseID + "."
	}
	current, err := listAPIKeysByName(ctx, r.client, req.Name)
	if err != nil {
		return detail + " Reconciliation could not list keys. No key was revoked. Inspect this name and organization and revoke any newly created key before retrying. " + apiKeyError(err)
	}
	newIDs := make(map[string]bool)
	responseMatches := false
	for _, key := range current {
		id := key.Id.String()
		if key.Id == uuid.Nil || knownIDs[id] || !apiKeyMatches(key, req.Name, req.OrgID) {
			continue
		}
		newIDs[id] = true
		if id == responseID {
			// When supplied, response ownership must agree with the listed key.
			responseMatches = (created.Name == "" || created.Name == key.Name) &&
				(nullableUUIDToTF(created.OrgId).IsNull() || nullableUUIDToTF(created.OrgId).Equal(nullableUUIDToTF(key.OrgId))) &&
				(nullableUUIDToTF(created.UserId).IsNull() || nullableUUIDToTF(created.UserId).Equal(nullableUUIDToTF(key.UserId)))
		}
	}
	ids := make([]string, 0, len(newIDs))
	for id := range newIDs {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	// A new name match alone may belong to a concurrent caller, not this POST.
	if len(ids) == 1 && responseMatches && responseID == ids[0] {
		_, err := client.Write(r.client.API.DeleteApiKeyIdWithResponse(ctx, responseID))
		if err == nil || client.IsNotFoundError(err) {
			return detail + " The uniquely reconciled new key (ID: " + responseID + ") was revoked or is already absent. Its raw key was not delivered; retry only after resolving the response failure."
		}
		return detail + " Cleanup could not confirm revocation of the uniquely reconciled new key (ID: " + responseID + "). Inspect and revoke this ID manually before retrying. " + apiKeyError(err)
	}
	if len(ids) == 0 {
		return detail + " No new matching key could be identified. No key was revoked. This does not prove the POST failed; inspect this name and organization and revoke any newly created key before retrying."
	}
	return detail + " Reconciliation was ambiguous; newly observed matching key IDs: " + strings.Join(ids, ", ") + ". No key was revoked. Inspect these IDs and revoke only keys created by this attempt before retrying."
}
