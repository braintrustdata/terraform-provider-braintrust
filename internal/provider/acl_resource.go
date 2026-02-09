package provider

import (
	"context"

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
	_ resource.Resource                = &aclResource{}
	_ resource.ResourceWithConfigure   = &aclResource{}
	_ resource.ResourceWithImportState = &aclResource{}
)

// NewACLResource returns a new ACL resource.
func NewACLResource() resource.Resource {
	return &aclResource{}
}

type aclResource struct {
	client *client.Client
}

func (r *aclResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_acl"
}

func (r *aclResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	requiresReplace := []planmodifier.String{stringplanmodifier.RequiresReplace()}

	resp.Schema = schema.Schema{
		Description: "Manages a Braintrust ACL (access control list) entry.",
		MarkdownDescription: "Manages a Braintrust ACL (access control list) entry. ACLs grant a permission or " +
			"role to a user or group on a specific object.\n\n" +
			"ACLs are **immutable** — any attribute change requires destroying and recreating the ACL. " +
			"ACLs are inherited through the object hierarchy (e.g., a project ACL applies to all experiments and datasets within it).",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "Unique identifier for the ACL (UUID).",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"object_type": schema.StringAttribute{
				Required:    true,
				Description: "The type of object the ACL applies to.",
				PlanModifiers: requiresReplace,
				Validators: []validator.String{
					stringvalidator.OneOf(aclObjectTypes...),
				},
			},
			"object_id": schema.StringAttribute{
				Required:    true,
				Description: "The ID of the object the ACL applies to.",
				PlanModifiers: requiresReplace,
			},
			"user_id": schema.StringAttribute{
				Optional:    true,
				Description: "ID of the user the ACL applies to. Exactly one of user_id and group_id must be provided.",
				PlanModifiers: requiresReplace,
				Validators: []validator.String{
					stringvalidator.ExactlyOneOf(
						path.MatchRoot("group_id"),
					),
				},
			},
			"group_id": schema.StringAttribute{
				Optional:    true,
				Description: "ID of the group the ACL applies to. Exactly one of user_id and group_id must be provided.",
				PlanModifiers: requiresReplace,
			},
			"permission": schema.StringAttribute{
				Optional:    true,
				Description: "The permission the ACL grants. Exactly one of permission and role_id must be provided.",
				PlanModifiers: requiresReplace,
				Validators: []validator.String{
					stringvalidator.ExactlyOneOf(
						path.MatchRoot("role_id"),
					),
					stringvalidator.OneOf(permissionTypes...),
				},
			},
			"restrict_object_type": schema.StringAttribute{
				Optional:    true,
				Description: "Optionally restricts the permission to a specific object type. Cannot be set when role_id is used.",
				PlanModifiers: requiresReplace,
				Validators: []validator.String{
					stringvalidator.OneOf(aclObjectTypes...),
					stringvalidator.ConflictsWith(
						path.MatchRoot("role_id"),
					),
				},
			},
			"role_id": schema.StringAttribute{
				Optional:    true,
				Description: "ID of the role the ACL grants. Exactly one of permission and role_id must be provided.",
				PlanModifiers: requiresReplace,
			},
			"object_org_id": schema.StringAttribute{
				Computed:    true,
				Description: "The organization the ACL's referenced object belongs to.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"created": schema.StringAttribute{
				Computed:    true,
				Description: "Date of ACL creation.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

func (r *aclResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = clientFromProviderData(req.ProviderData, &resp.Diagnostics)
}

func (r *aclResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan aclModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !requireClient(r.client, &resp.Diagnostics) {
		return
	}

	createReq := client.CreateACLRequest{
		ObjectType:         plan.ObjectType.ValueString(),
		ObjectID:           plan.ObjectID.ValueString(),
		UserID:             tfStringToPtr(plan.UserID),
		GroupID:            tfStringToPtr(plan.GroupID),
		Permission:         tfStringToPtr(plan.Permission),
		RestrictObjectType: tfStringToPtr(plan.RestrictObjectType),
		RoleID:             tfStringToPtr(plan.RoleID),
	}

	tflog.Debug(ctx, "Creating ACL", map[string]any{
		"object_type": createReq.ObjectType,
		"object_id":   createReq.ObjectID,
	})

	acl, err := r.client.CreateACL(ctx, createReq)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Creating Braintrust ACL",
			"Could not create ACL, unexpected error: "+err.Error(),
		)
		return
	}

	mapACLToModel(acl, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *aclResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state aclModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !requireClient(r.client, &resp.Diagnostics) {
		return
	}

	acl, err := r.client.GetACL(ctx, state.ID.ValueString())
	if err != nil {
		if client.IsNotFoundError(err) {
			tflog.Warn(ctx, "ACL not found, removing from state", map[string]any{
				"id": state.ID.ValueString(),
			})
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Error Reading Braintrust ACL",
			"Could not read ACL ID "+state.ID.ValueString()+": "+err.Error(),
		)
		return
	}

	mapACLToModel(acl, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

// Update is not supported for ACLs -- all attributes have RequiresReplace.
// The framework should never call this, but we implement it to satisfy the interface.
func (r *aclResource) Update(_ context.Context, _ resource.UpdateRequest, resp *resource.UpdateResponse) {
	resp.Diagnostics.AddError(
		"ACL Update Not Supported",
		"ACLs are immutable. This is a provider bug — the framework should have triggered a replace instead.",
	)
}

func (r *aclResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state aclModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !requireClient(r.client, &resp.Diagnostics) {
		return
	}

	err := r.client.DeleteACL(ctx, state.ID.ValueString())
	if err != nil {
		if client.IsNotFoundError(err) {
			return
		}
		resp.Diagnostics.AddError(
			"Error Deleting Braintrust ACL",
			"Could not delete ACL ID "+state.ID.ValueString()+": "+err.Error(),
		)
	}
}

func (r *aclResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
