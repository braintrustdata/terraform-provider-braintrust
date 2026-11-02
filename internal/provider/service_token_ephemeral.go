package provider

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/braintrustdata/braintrust-sdk-go/openapi"
	"github.com/braintrustdata/terraform-provider-braintrust/internal/client"
	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/ephemeral"
	"github.com/hashicorp/terraform-plugin-framework/ephemeral/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/oapi-codegen/nullable"
)

var (
	_ ephemeral.EphemeralResource              = &serviceTokenEphemeralResource{}
	_ ephemeral.EphemeralResourceWithConfigure = &serviceTokenEphemeralResource{}
)

// NewServiceTokenEphemeralResource returns a state-free service token generator.
func NewServiceTokenEphemeralResource() ephemeral.EphemeralResource {
	return &serviceTokenEphemeralResource{}
}

type serviceTokenEphemeralResource struct {
	client *client.Client
}

type serviceTokenEphemeralModel struct {
	ID               types.String `tfsdk:"id"`
	Name             types.String `tfsdk:"name"`
	ServiceAccountID types.String `tfsdk:"service_account_id"`
	ExpiresInSeconds types.Int64  `tfsdk:"expires_in_seconds"`
	Nonce            types.String `tfsdk:"nonce"`
	Key              types.String `tfsdk:"key"`
}

func (r *serviceTokenEphemeralResource) Metadata(_ context.Context, req ephemeral.MetadataRequest, resp *ephemeral.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_service_token"
}

func (r *serviceTokenEphemeralResource) Schema(_ context.Context, _ ephemeral.SchemaRequest, resp *ephemeral.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Generates a Braintrust service token without storing its data in Terraform plan or state files.",
		MarkdownDescription: "Generates a Braintrust service token without storing its data in Terraform plan or state files.\n\n" +
			"**Ephemeral describes the Terraform data, not the remote token lifetime.** The provider does not revoke the token when Terraform closes the ephemeral resource or destroys the configuration, and does not automatically rotate it. " +
			"Callers own durable delivery, store lookup, rotation, expiration, and revocation. The raw `key` is returned only at creation and cannot be recovered from the API.\n\n" +
			"**Set `nonce = timestamp()` (or `uuid()`) so planning does not mint a token.** Terraform opens ephemeral resources during plan when every input is known; those functions are unknown until apply, which defers Open. A literal `nonce` mints a token on every plan. " +
			"Each Open creates a new token, so use a unique name within the service account. Existing same-name tokens are rejected, never adopted.\n\n" +
			"Creation requires an organization-owner service token. When the caller token expires, `expires_in_seconds` must be less than its remaining lifetime.\n\n" +
			"If a create response is lost or incomplete, the provider does not retry creation. It attempts revocation only when the create response identifies the sole newly reconciled matching token. Without a response ID, even a single new name match may belong to another caller and is never automatically revoked; the error identifies any known IDs for manual inspection and revocation. " +
			"No recovery ID is retained in Terraform state. A successful Open followed by lost downstream delivery cannot be automatically detected or recovered; inspect and revoke that token before minting a replacement.",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				Required:    true,
				Description: "Name of the token. Must be unique within its service account for safe creation and recovery.",
				Validators:  []validator.String{stringvalidator.LengthAtLeast(1)},
			},
			"service_account_id": schema.StringAttribute{
				Required:    true,
				Description: "ID of the existing service account to which this token belongs.",
				Validators:  []validator.String{stringvalidator.LengthAtLeast(1)},
			},
			"nonce": schema.StringAttribute{
				Required:    true,
				Description: "Set to timestamp() or uuid(). Not sent to the API; its value is unknown during planning, so Terraform defers minting until apply. A literal value mints a token during plan.",
				Validators:  []validator.String{stringvalidator.LengthAtLeast(1)},
			},
			"expires_in_seconds": schema.Int64Attribute{
				Optional:    true,
				Description: "Positive token lifetime in seconds. Required when minting with an expiring owner token and must be less than its remaining lifetime. Omit for no expiration when permitted by the API.",
				Validators:  []validator.Int64{int64validator.AtLeast(1)},
			},
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "ID of the created token. Deliver it with the key so the token can later be revoked.",
			},
			"key": schema.StringAttribute{
				Computed:    true,
				Sensitive:   true,
				Description: "Raw token, available only at creation. Never stored in Terraform plan or state files; callers must deliver it securely.",
			},
		},
	}
}

func (r *serviceTokenEphemeralResource) Configure(_ context.Context, req ephemeral.ConfigureRequest, resp *ephemeral.ConfigureResponse) {
	r.client = clientFromProviderData(req.ProviderData, &resp.Diagnostics)
}

func (r *serviceTokenEphemeralResource) Open(ctx context.Context, req ephemeral.OpenRequest, resp *ephemeral.OpenResponse) {
	var config serviceTokenEphemeralModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() || !requireClient(r.client, &resp.Diagnostics) {
		return
	}
	if config.Name.IsUnknown() || config.Name.IsNull() || config.Name.ValueString() == "" ||
		config.ServiceAccountID.IsUnknown() || config.ServiceAccountID.IsNull() || config.ServiceAccountID.ValueString() == "" ||
		config.Nonce.IsUnknown() || config.Nonce.IsNull() || config.Nonce.ValueString() == "" ||
		config.ExpiresInSeconds.IsUnknown() {
		resp.Diagnostics.AddError("Invalid Service Token Configuration", "name, service_account_id, and nonce must be known and nonempty, and expires_in_seconds must be known or omitted before opening the ephemeral resource.")
		return
	}
	createReq := openapi.PostServiceTokenJSONRequestBody{
		Name:             config.Name.ValueString(),
		ServiceAccountId: config.ServiceAccountID.ValueString(),
	}
	if !config.ExpiresInSeconds.IsNull() {
		seconds := config.ExpiresInSeconds.ValueInt64()
		if seconds <= 0 || int64(int(seconds)) != seconds {
			resp.Diagnostics.AddError("Invalid Service Token Lifetime", "expires_in_seconds must be a positive integer representable by the API client.")
			return
		}
		createReq.ExpiresInSeconds = nullable.NewNullableWithValue(int(seconds))
	}

	existing, err := listServiceTokensByName(ctx, r.client, createReq.Name)
	if err != nil {
		resp.Diagnostics.AddError("Error Checking Existing Service Tokens", "No token was created. "+serviceTokenEphemeralError(err))
		return
	}
	knownIDs := make(map[string]bool, len(existing))
	for _, token := range existing {
		knownIDs[token.Id.String()] = true
		if serviceTokenMatches(token, createReq.Name, createReq.ServiceAccountId) {
			resp.Diagnostics.AddError("Service Token Name Already Used", "A token with this name and service account already exists (ID: "+token.Id.String()+"). Its raw key cannot be recovered. Use a unique name or revoke that token before creating another; it was not adopted or deleted.")
			return
		}
	}

	// Never repeat an uncertain POST: the raw key cannot be retrieved later.
	created, err := client.Write(r.client.API.PostServiceTokenWithResponse(ctx, createReq))
	var output *openapi.CreateServiceTokenOutput
	if err == nil {
		output, err = client.Written(created.JSON200)
	}
	responseID := ""
	if output != nil {
		if output.Id != uuid.Nil {
			responseID = output.Id.String()
		}
		if responseID == "" || output.Key == "" {
			err = &client.UncertainWriteError{Method: http.MethodPost, Cause: errors.New("create response omitted the token ID or raw key")}
		}
	}
	if err != nil {
		if client.IsUncertainWriteError(err) || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			// ctx may be the reason the POST failed; cleanup must still run.
			cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), serviceTokenCleanupTimeout)
			defer cancel()
			resp.Diagnostics.AddError("Service Token Creation Outcome Uncertain", r.reconcileUncertainOpen(cleanupCtx, createReq, knownIDs, responseID)+" "+serviceTokenEphemeralError(err))
		} else {
			resp.Diagnostics.AddError("Error Creating Braintrust Service Token", "Token creation failed. "+serviceTokenEphemeralError(err))
		}
		return
	}
	if knownIDs[responseID] {
		resp.Diagnostics.AddError("Service Token Was Not Newly Created", "The API returned a preexisting token ID: "+responseID+". It was not adopted or deleted. Inspect the remote tokens before retrying.")
		return
	}

	// Keep the configured inputs verbatim, even if the API normalizes them.
	config.ID = types.StringValue(responseID)
	config.Key = types.StringValue(output.Key)
	resp.Diagnostics.Append(resp.Result.Set(ctx, config)...)
}

// serviceTokenCleanupTimeout bounds reconciliation after an interrupted create.
const serviceTokenCleanupTimeout = 30 * time.Second

func (r *serviceTokenEphemeralResource) reconcileUncertainOpen(ctx context.Context, req openapi.PostServiceTokenJSONRequestBody, knownIDs map[string]bool, responseID string) string {
	detail := "The POST was not retried, and no recovery ID is retained in Terraform state."
	if responseID != "" {
		detail += " The create response identified token ID: " + responseID + "."
	}
	current, err := listServiceTokensByName(ctx, r.client, req.Name)
	if err != nil {
		return detail + " Reconciliation could not list tokens. No token was revoked. Inspect this name and service account and revoke any newly created token before retrying. " + serviceTokenEphemeralError(err)
	}
	newIDs := make(map[string]bool)
	for _, token := range current {
		id := token.Id.String()
		if token.Id != uuid.Nil && !knownIDs[id] && serviceTokenMatches(token, req.Name, req.ServiceAccountId) {
			newIDs[id] = true
		}
	}
	ids := make([]string, 0, len(newIDs))
	for id := range newIDs {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	// A new name match may belong to a concurrent caller. Only the create
	// response can attribute the candidate to this attempt.
	if len(ids) == 1 && responseID != "" && responseID == ids[0] {
		id := ids[0]
		_, err := client.Write(r.client.API.DeleteServiceTokenIdWithResponse(ctx, id))
		if err == nil || client.IsNotFoundError(err) {
			return detail + " The uniquely reconciled new token (ID: " + id + ") was revoked or is already absent. Its raw key was not delivered; retry only after resolving the response failure."
		}
		return detail + " Cleanup could not confirm revocation of the uniquely reconciled new token (ID: " + id + "). Inspect and revoke this ID manually before retrying. " + serviceTokenEphemeralError(err)
	}
	if len(ids) == 0 {
		return detail + " No new matching token could be identified. No token was revoked. This does not prove the POST failed; inspect this name and service account and revoke any newly created token before retrying."
	}
	return detail + " Reconciliation was ambiguous; newly observed matching token IDs: " + strings.Join(ids, ", ") + ". No token was revoked. Inspect these IDs and revoke only tokens created by this attempt before retrying."
}

// API and decoding errors may contain the raw response/key. Never include their
// Error strings in diagnostics; retain only safe classifications and HTTP status.
func serviceTokenEphemeralError(err error) string {
	var apiErr *client.APIError
	if errors.As(err, &apiErr) {
		return fmt.Sprintf("Braintrust API returned HTTP %d; response details are omitted to protect credentials.", apiErr.StatusCode)
	}
	if errors.Is(err, context.Canceled) {
		return "The request was canceled."
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "The request deadline was exceeded."
	}
	if client.IsUncertainWriteError(err) {
		return "The write response was unavailable, incomplete, or could not be decoded; response details are omitted to protect credentials."
	}
	return "The API request failed or its response could not be decoded; response details are omitted to protect credentials."
}
