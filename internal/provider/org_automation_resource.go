package provider

import (
	"context"

	"github.com/braintrustdata/terraform-provider-braintrust/internal/client"
	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource                = &orgAutomationResource{}
	_ resource.ResourceWithConfigure   = &orgAutomationResource{}
	_ resource.ResourceWithImportState = &orgAutomationResource{}
)

func NewOrgAutomationResource() resource.Resource { return &orgAutomationResource{} }

type orgAutomationResource struct{ client *client.Client }

type orgAutomationModel struct {
	ID              types.String          `tfsdk:"id"`
	OrgID           types.String          `tfsdk:"org_id"`
	Name            types.String          `tfsdk:"name"`
	Description     types.String          `tfsdk:"description"`
	RetentionConfig *retentionConfigModel `tfsdk:"retention_config"`
	UserID          types.String          `tfsdk:"user_id"`
	Created         types.String          `tfsdk:"created"`
}

func (r *orgAutomationResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_org_automation"
}

func (r *orgAutomationResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages an organization-wide Braintrust retention policy. Available only on plans that permit custom retention.",
		Attributes: map[string]schema.Attribute{
			"id":          computedStringAttribute("Unique identifier for the organization automation."),
			"org_id":      schema.StringAttribute{Required: true, Description: "Organization this policy applies to.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"name":        schema.StringAttribute{Required: true, Description: "Name of the retention policy.", Validators: []validator.String{stringvalidator.LengthAtLeast(1)}},
			"description": schema.StringAttribute{Optional: true, Description: "Optional description of the policy."},
			"retention_config": schema.SingleNestedAttribute{
				Required:    true,
				Description: "Organization-wide retention period for one object type. At most one policy per object type can exist in an organization.",
				Attributes: map[string]schema.Attribute{
					"object_type": schema.StringAttribute{
						Required:      true,
						Description:   "The retained object type: project_logs, experiment, or dataset.",
						Validators:    []validator.String{stringvalidator.OneOf("project_logs", "experiment", "dataset")},
						PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
					},
					"retention_days": schema.Int64Attribute{
						Required:    true,
						Description: "Number of days to retain the object; zero is permitted by the API.",
						Validators:  []validator.Int64{int64validator.AtLeast(0)},
					},
				},
			},
			"user_id": computedStringAttribute("User who created the policy."),
			"created": computedStringAttribute("Date of policy creation."),
		},
	}
}

func (r *orgAutomationResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = clientFromProviderData(req.ProviderData, &resp.Diagnostics)
}

func orgRetentionFromModel(model *orgAutomationModel) client.OrgRetentionConfig {
	return client.OrgRetentionConfig{
		EventType: "retention", ObjectType: model.RetentionConfig.ObjectType.ValueString(),
		RetentionDays: model.RetentionConfig.RetentionDays.ValueInt64(),
	}
}

func mapOrgAutomationToModel(automation *client.OrgAutomation, model *orgAutomationModel) {
	model.ID = types.StringValue(automation.ID)
	model.OrgID = types.StringValue(automation.OrgID)
	model.Name = types.StringValue(automation.Name)
	model.Description = stringPtrToTFValue(automation.Description)
	model.UserID = stringPtrToTFValue(automation.UserID)
	model.Created = stringPtrToTFValue(automation.Created)
	model.RetentionConfig = &retentionConfigModel{
		ObjectType:    types.StringValue(automation.Config.ObjectType),
		RetentionDays: types.Int64Value(automation.Config.RetentionDays),
	}
}

func (r *orgAutomationResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan orgAutomationModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() || !requireClient(r.client, &resp.Diagnostics) {
		return
	}
	created, err := r.client.CreateOrgAutomation(ctx, client.CreateOrgAutomationRequest{
		OrgID: plan.OrgID.ValueString(), Name: plan.Name.ValueString(),
		Description: tfStringToPtr(plan.Description), Config: orgRetentionFromModel(&plan),
	})
	if err != nil {
		resp.Diagnostics.AddError("Error Creating Braintrust Organization Automation", err.Error())
		return
	}
	// Preserve the ID if a follow-up read fails so Terraform can manage cleanup.
	automation, err := r.client.GetOrgAutomation(ctx, created.ID)
	if err != nil {
		mapOrgAutomationToModel(created, &plan)
		resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
		resp.Diagnostics.AddError("Error Reading Organization Automation After Create", "Created ID "+created.ID+": "+err.Error())
		return
	}
	mapOrgAutomationToModel(automation, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *orgAutomationResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state orgAutomationModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() || !requireClient(r.client, &resp.Diagnostics) {
		return
	}
	automation, err := r.client.GetOrgAutomation(ctx, state.ID.ValueString())
	if err != nil {
		if client.IsNotFoundError(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error Reading Braintrust Organization Automation", err.Error())
		return
	}
	mapOrgAutomationToModel(automation, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *orgAutomationResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state orgAutomationModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() || !requireClient(r.client, &resp.Diagnostics) {
		return
	}
	automation, err := r.client.UpdateOrgAutomation(ctx, state.ID.ValueString(), client.UpdateOrgAutomationRequest{
		Name: plan.Name.ValueString(), Description: tfStringToPtr(plan.Description), Config: orgRetentionFromModel(&plan),
	})
	if err != nil {
		resp.Diagnostics.AddError("Error Updating Braintrust Organization Automation", err.Error())
		return
	}
	mapOrgAutomationToModel(automation, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *orgAutomationResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state orgAutomationModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() || !requireClient(r.client, &resp.Diagnostics) {
		return
	}
	if err := r.client.DeleteOrgAutomation(ctx, state.ID.ValueString()); err != nil && !client.IsNotFoundError(err) {
		resp.Diagnostics.AddError("Error Deleting Braintrust Organization Automation", err.Error())
	}
}

func (r *orgAutomationResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
