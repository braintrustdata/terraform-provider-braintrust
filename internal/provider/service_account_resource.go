package provider

import (
	"context"
	"fmt"

	"github.com/braintrustdata/terraform-provider-braintrust/internal/client"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

var (
	_ resource.Resource              = &serviceAccountResource{}
	_ resource.ResourceWithConfigure = &serviceAccountResource{}
)

// NewServiceAccountResource returns a new service account resource.
func NewServiceAccountResource() resource.Resource {
	return &serviceAccountResource{}
}

type serviceAccountResource struct {
	client *client.Client
}

func (r *serviceAccountResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_service_account"
}

func (r *serviceAccountResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Version:     0,
		Description: "Manages a Braintrust service account.",
		MarkdownDescription: "Manages a Braintrust service account.\n\n" +
			"Service accounts are organization members intended for non-human, programmatic use. " +
			"Use `braintrust_service_token` to mint tokens for this service account. " +
			"The current public API does not support reading service accounts, so Terraform preserves the last known state during refresh and cannot detect external deletion or drift.",
		Attributes: map[string]schema.Attribute{
			"id":      computedStringAttribute("Unique identifier for the service account (UUID)."),
			"email":   computedStringAttribute("The service account email (not routable)."),
			"created": computedStringAttribute("Date of service account creation, if exposed by the API. Currently null."),
			"name": schema.StringAttribute{
				Required:    true,
				Description: "Name of the service account.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
		},
	}
}

func (r *serviceAccountResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = clientFromProviderData(req.ProviderData, &resp.Diagnostics)
}

func (r *serviceAccountResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan serviceAccountModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !requireClient(r.client, &resp.Diagnostics) {
		return
	}

	createReq := client.PatchOrganizationMembersRequest{
		InviteUsers: &client.InviteUsersRequest{
			ServiceAccounts: []client.ServiceAccountInvite{
				{Name: plan.Name.ValueString()},
			},
		},
	}

	tflog.Debug(ctx, "Creating service account", map[string]any{"name": plan.Name.ValueString()})

	output, err := r.client.PatchOrganizationMembers(ctx, createReq)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Creating Braintrust Service Account",
			"Could not create service account, unexpected error: "+err.Error(),
		)
		return
	}

	if len(output.AddedUsers) == 0 {
		resp.Diagnostics.AddError(
			"Error Creating Braintrust Service Account",
			"Service account creation succeeded but no created member was returned by the API response.",
		)
		return
	}

	created := output.AddedUsers[0]
	plan.ID = types.StringValue(created.ID)
	plan.Email = stringPtrToTFValue(created.Email)
	// The membership response does not expose a creation timestamp, and the
	// public user endpoint excludes service accounts.
	plan.Created = types.StringNull()
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *serviceAccountResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state serviceAccountModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !requireClient(r.client, &resp.Diagnostics) {
		return
	}

	// GET /v1/user is restricted to human users. An empty result for a
	// service-account ID is not evidence of deletion. Retain the known state
	// until the API offers a service-account read contract.
	resp.Diagnostics.AddWarning("Service Account Refresh Is Limited", "Braintrust does not expose a public service-account read endpoint. Terraform preserves the last known service-account state; deletion or drift outside Terraform cannot currently be detected.")
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

// Update is not supported -- all mutable attributes use RequiresReplace.
func (r *serviceAccountResource) Update(_ context.Context, _ resource.UpdateRequest, resp *resource.UpdateResponse) {
	resp.Diagnostics.AddError(
		"Service Account Update Not Supported",
		"Service accounts are immutable in this provider. This is a provider bug — the framework should have triggered a replace instead.",
	)
}

func (r *serviceAccountResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state serviceAccountModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !requireClient(r.client, &resp.Diagnostics) {
		return
	}

	deleteReq := client.PatchOrganizationMembersRequest{
		RemoveUsers: &client.RemoveUsersRequest{
			IDs: []string{state.ID.ValueString()},
		},
	}

	_, err := r.client.PatchOrganizationMembers(ctx, deleteReq)
	if err != nil {
		if client.IsNotFoundError(err) {
			return
		}
		resp.Diagnostics.AddError(
			"Error Deleting Braintrust Service Account",
			fmt.Sprintf("Could not delete service account ID %s: %s", state.ID.ValueString(), err.Error()),
		)
	}
}
