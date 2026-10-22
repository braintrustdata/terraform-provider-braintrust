package provider

import (
	"context"

	"github.com/braintrustdata/braintrust-sdk-go/openapi"
	"github.com/braintrustdata/terraform-provider-braintrust/internal/client"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/oapi-codegen/nullable"
)

var (
	_ resource.Resource              = &serviceTokenResource{}
	_ resource.ResourceWithConfigure = &serviceTokenResource{}
)

// NewServiceTokenResource returns a new service token resource.
func NewServiceTokenResource() resource.Resource {
	return &serviceTokenResource{}
}

type serviceTokenResource struct {
	client *client.Client
}

func (r *serviceTokenResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_service_token"
}

func (r *serviceTokenResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Version:     0,
		Description: "Manages a Braintrust service token.",
		MarkdownDescription: "Manages a Braintrust service token.\n\n" +
			"Service tokens are scoped to a service account and are used for programmatic API access.\n\n" +
			"Creation requires an organization-owner service token. If the caller token expires, set `expires_in_seconds` below its remaining lifetime.\n\n" +
			"**Important:** The raw token (`key`) is only exposed at creation time and stored in Terraform state. " +
			"It cannot be retrieved again from the API. Treat the Terraform state as sensitive.\n\n" +
			"Service tokens are **immutable** — any attribute change requires destroying and recreating the token. " +
			"Import is not supported because the raw key cannot be recovered. Give each Terraform-managed token a unique name within its service account. " +
			"If a create response is lost, the provider records any newly discovered token ID as tainted state; the next apply revokes it before recreating the token.",
		Attributes: map[string]schema.Attribute{
			"id":                    computedStringAttribute("Unique identifier for the service token (UUID)."),
			"preview_name":          computedStringAttribute("A preview/masked version of the token name."),
			"created":               computedStringAttribute("Date of service token creation."),
			"org_id":                computedStringAttribute("Unique identifier for the organization."),
			"service_account_email": computedStringAttribute("The service account email (not routable)."),
			"service_account_name":  computedStringAttribute("The service account name."),
			"expires_at":            computedStringAttribute("Timestamp when the token expires, if configured."),
			"expires_in_seconds": schema.Int64Attribute{
				Optional:      true,
				Description:   "Token lifetime in seconds. Required when minting with an expiring owner service token; it must not exceed the caller token's remaining lifetime.",
				PlanModifiers: []planmodifier.Int64{int64planmodifier.RequiresReplace()},
			},
			"name": schema.StringAttribute{
				Required:    true,
				Description: "Name of the service token. Terraform requires it to be unique within its service account for safe create recovery.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"service_account_id": schema.StringAttribute{
				Required:    true,
				Description: "The service account ID this service token should belong to.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"key": schema.StringAttribute{
				Computed:    true,
				Sensitive:   true,
				Description: "The raw service token. Only exposed at creation time and stored in Terraform state.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

func (r *serviceTokenResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = clientFromProviderData(req.ProviderData, &resp.Diagnostics)
}

func (r *serviceTokenResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan serviceTokenModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !requireClient(r.client, &resp.Diagnostics) {
		return
	}

	createReq := openapi.PostServiceTokenJSONRequestBody{
		Name:             plan.Name.ValueString(),
		ServiceAccountId: plan.ServiceAccountID.ValueString(),
	}
	if !plan.ExpiresInSeconds.IsNull() && !plan.ExpiresInSeconds.IsUnknown() {
		seconds := plan.ExpiresInSeconds.ValueInt64()
		if seconds <= 0 {
			resp.Diagnostics.AddError("Invalid Service Token Lifetime", "expires_in_seconds must be greater than zero")
			return
		}
		createReq.ExpiresInSeconds = nullable.NewNullableWithValue(int(seconds))
	}

	tflog.Debug(ctx, "Creating service token", map[string]any{
		"name":               createReq.Name,
		"service_account_id": createReq.ServiceAccountId,
	})
	// The API exposes the raw token only in the POST response. If that response
	// is lost, issuing the POST again would mint a second credential. Reserve a
	// unique name per account and use the list endpoint to reconcile the result.
	existing, err := listServiceTokensByName(ctx, r.client, createReq.Name)
	if err != nil {
		resp.Diagnostics.AddError("Error Checking Existing Service Tokens", err.Error())
		return
	}
	knownIDs := make(map[string]bool)
	for _, token := range existing {
		knownIDs[token.Id.String()] = true
		if serviceTokenMatches(token, createReq.Name, createReq.ServiceAccountId) {
			resp.Diagnostics.AddError("Service Token Name Already Used", "A token with this name and service account already exists (ID: "+token.Id.String()+"). Use a unique name or remove that token before creating another. Its raw key cannot be recovered through the API.")
			return
		}
	}

	createResp, err := client.Write(r.client.API.PostServiceTokenWithResponse(ctx, createReq))
	var output *openapi.CreateServiceTokenOutput
	if err == nil {
		output, err = client.JSON(createResp.JSON200)
	}
	if err != nil {
		if client.IsUncertainWriteError(err) {
			current, listErr := listServiceTokensByName(ctx, r.client, createReq.Name)
			if listErr == nil {
				var newTokens []openapi.ServiceToken
				for _, token := range current {
					if !knownIDs[token.Id.String()] && serviceTokenMatches(token, createReq.Name, createReq.ServiceAccountId) {
						newTokens = append(newTokens, token)
					}
				}
				if len(newTokens) == 1 {
					mapServiceTokenToModel(&newTokens[0], &plan)
					plan.Key = types.StringNull()
					resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
					resp.Diagnostics.AddError("Service Token Created But Key Was Lost", "The POST outcome was uncertain, but a new token (ID: "+newTokens[0].Id.String()+") appeared. Its raw key cannot be recovered. Terraform retained the ID in tainted state and will delete it before recreating the token on the next apply. Original error: "+err.Error())
					return
				}
			}
		}
		resp.Diagnostics.AddError(
			"Error Creating Braintrust Service Token",
			"Could not create service token, unexpected error: "+err.Error(),
		)
		return
	}

	// Map from the create response (the only time we get the raw key).
	mapCreateServiceTokenOutputToModel(output, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *serviceTokenResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state serviceTokenModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !requireClient(r.client, &resp.Diagnostics) {
		return
	}

	token, err := getServiceToken(ctx, r.client, state.ID.ValueString())
	if err != nil {
		if client.IsNotFoundError(err) {
			tflog.Warn(ctx, "Service token not found, removing from state", map[string]any{
				"id": state.ID.ValueString(),
			})
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Error Reading Braintrust Service Token",
			"Could not read service token ID "+state.ID.ValueString()+": "+err.Error(),
		)
		return
	}

	// mapServiceTokenToModel preserves the existing state.Key (never returned by GET).
	mapServiceTokenToModel(token, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

// Update is not supported -- all mutable attributes use RequiresReplace.
func (r *serviceTokenResource) Update(_ context.Context, _ resource.UpdateRequest, resp *resource.UpdateResponse) {
	resp.Diagnostics.AddError(
		"Service Token Update Not Supported",
		"Service tokens are immutable. This is a provider bug — the framework should have triggered a replace instead.",
	)
}

func (r *serviceTokenResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state serviceTokenModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !requireClient(r.client, &resp.Diagnostics) {
		return
	}

	_, err := client.Write(r.client.API.DeleteServiceTokenIdWithResponse(ctx, state.ID.ValueString()))
	if err != nil {
		if client.IsNotFoundError(err) {
			return
		}
		resp.Diagnostics.AddError(
			"Error Deleting Braintrust Service Token",
			"Could not delete service token ID "+state.ID.ValueString()+": "+err.Error(),
		)
	}
}
