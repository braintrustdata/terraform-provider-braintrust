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
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/oapi-codegen/nullable"
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
						Description:   "The retained object type. " + oneOfDescription("project_logs", "experiment", "dataset"),
						Validators:    []validator.String{stringvalidator.OneOf("project_logs", "experiment", "dataset")},
						PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
					},
					"retention_days": schema.Int64Attribute{
						Required:    true,
						Description: "Number of days to retain data. 0 means delete immediately.",
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

// These match the SDK's inline retention config structs, so values can be
// assigned to the request fields directly.
type (
	orgRetentionCreateConfig = struct {
		EventType     openapi.CreateOrgAutomationConfigEventType `json:"event_type"`
		ObjectType    openapi.RetentionObjectType                `json:"object_type"`
		RetentionDays int                                        `json:"retention_days"`
	}
	orgRetentionPatchConfig = struct {
		EventType     openapi.PatchOrgAutomationConfigEventType `json:"event_type"`
		ObjectType    openapi.RetentionObjectType               `json:"object_type"`
		RetentionDays int                                       `json:"retention_days"`
	}
)

func orgRetentionCreateFromModel(model *orgAutomationModel) orgRetentionCreateConfig {
	return orgRetentionCreateConfig{
		EventType:     openapi.CreateOrgAutomationConfigEventTypeRetention,
		ObjectType:    openapi.RetentionObjectType(model.RetentionConfig.ObjectType.ValueString()),
		RetentionDays: int(model.RetentionConfig.RetentionDays.ValueInt64()),
	}
}

func orgRetentionPatchFromModel(model *orgAutomationModel) orgRetentionPatchConfig {
	return orgRetentionPatchConfig{
		EventType:     openapi.PatchOrgAutomationConfigEventTypeRetention,
		ObjectType:    openapi.RetentionObjectType(model.RetentionConfig.ObjectType.ValueString()),
		RetentionDays: int(model.RetentionConfig.RetentionDays.ValueInt64()),
	}
}

func mapOrgAutomationToModel(automation *openapi.OrgAutomation, model *orgAutomationModel) {
	model.ID = types.StringValue(automation.Id.String())
	model.OrgID = types.StringValue(automation.OrgId.String())
	model.Name = types.StringValue(automation.Name)
	model.Description = nullableStringToTF(automation.Description)
	model.UserID = nullableUUIDToTF(automation.UserId)
	model.Created = nullableTimeToTF(automation.Created, model.Created)
	model.RetentionConfig = &retentionConfigModel{
		ObjectType:    types.StringValue(string(automation.Config.ObjectType)),
		RetentionDays: types.Int64Value(int64(automation.Config.RetentionDays)),
	}
}

func getOrgAutomation(ctx context.Context, c *client.Client, id string) (*openapi.OrgAutomation, error) {
	resp, err := c.API.GetOrgAutomationIdWithResponse(ctx, id)
	if err != nil {
		return nil, err
	}
	return client.JSON(resp.JSON200)
}

func (r *orgAutomationResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan orgAutomationModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() || !requireClient(r.client, &resp.Diagnostics) {
		return
	}
	orgID, err := parseUUID(plan.OrgID, "org_id")
	if err != nil {
		resp.Diagnostics.AddAttributeError(path.Root("org_id"), "Invalid Organization ID", err.Error())
		return
	}
	createResp, err := client.Write(r.client.API.PostOrgAutomationWithResponse(ctx, openapi.CreateOrgAutomation{
		OrgId: orgID, Name: plan.Name.ValueString(),
		Description: tfStringToNullable(plan.Description), Config: orgRetentionCreateFromModel(&plan),
	}))
	var created *openapi.OrgAutomation
	if err == nil {
		created, err = client.Created(createResp.HTTPResponse, createResp.JSON200, "organization automation")
	}
	if err != nil {
		resp.Diagnostics.AddError("Error Creating Braintrust Organization Automation", err.Error())
		return
	}
	// Preserve the ID if a follow-up read fails so Terraform can manage cleanup.
	automation, err := getOrgAutomation(ctx, r.client, created.Id.String())
	if err != nil {
		mapOrgAutomationToModel(created, &plan)
		resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
		resp.Diagnostics.AddError("Error Reading Organization Automation After Create", "Created ID "+created.Id.String()+": "+err.Error())
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
	automation, err := getOrgAutomation(ctx, r.client, state.ID.ValueString())
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
	// Description is always sent; null clears it.
	description := nullable.NewNullNullable[string]()
	if !plan.Description.IsNull() && !plan.Description.IsUnknown() {
		description = nullable.NewNullableWithValue(plan.Description.ValueString())
	}
	updateResp, err := client.Write(r.client.API.PatchOrgAutomationIdWithResponse(ctx, state.ID.ValueString(), openapi.PatchOrgAutomation{
		Name: nullable.NewNullableWithValue(plan.Name.ValueString()), Description: description,
		Config: nullable.NewNullableWithValue(orgRetentionPatchFromModel(&plan)),
	}))
	var automation *openapi.OrgAutomation
	if err == nil {
		automation, err = client.Written(updateResp.JSON200)
	}
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
	if _, err := client.Write(r.client.API.DeleteOrgAutomationIdWithResponse(ctx, state.ID.ValueString())); err != nil && !client.IsNotFoundError(err) {
		resp.Diagnostics.AddError("Error Deleting Braintrust Organization Automation", err.Error())
	}
}

func (r *orgAutomationResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
