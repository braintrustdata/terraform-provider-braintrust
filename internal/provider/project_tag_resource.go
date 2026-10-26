package provider

import (
	"context"

	"github.com/braintrustdata/braintrust-sdk-go/openapi"
	"github.com/braintrustdata/terraform-provider-braintrust/internal/client"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/oapi-codegen/nullable"
)

var (
	_ resource.Resource                = &projectTagResource{}
	_ resource.ResourceWithConfigure   = &projectTagResource{}
	_ resource.ResourceWithImportState = &projectTagResource{}
)

// NewProjectTagResource returns a new project tag resource.
func NewProjectTagResource() resource.Resource {
	return &projectTagResource{}
}

type projectTagResource struct {
	client *client.Client
}

func (r *projectTagResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_project_tag"
}

func (r *projectTagResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Version:             0,
		Description:         "Manages a Braintrust project tag.",
		MarkdownDescription: "Manages a Braintrust project tag. Tags are user-configured labels for tracking and filtering experiments, logs, and other data within a project.",
		Attributes: map[string]schema.Attribute{
			"id":      computedStringAttribute("Unique identifier for the project tag (UUID)."),
			"user_id": computedStringAttribute("Identifies the user who created the project tag."),
			"created": computedStringAttribute("Date of project tag creation."),
			"project_id": schema.StringAttribute{
				Required:    true,
				Description: "Unique identifier for the project that the tag belongs to.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"name": schema.StringAttribute{
				Required:    true,
				Description: "Name of the project tag.",
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"description": optionalComputedStringAttribute("Textual description of the project tag."),
			"color": schema.StringAttribute{
				Optional:    true,
				Description: "Color of the tag for the UI.",
			},
		},
	}
}

func (r *projectTagResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = clientFromProviderData(req.ProviderData, &resp.Diagnostics)
}

func (r *projectTagResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan projectTagModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !requireClient(r.client, &resp.Diagnostics) {
		return
	}

	projectID, err := parseUUID(plan.ProjectID, "project_id")
	if err != nil {
		resp.Diagnostics.AddAttributeError(path.Root("project_id"), "Invalid Project ID", err.Error())
		return
	}
	createReq := openapi.CreateProjectTag{
		ProjectId:   projectID,
		Name:        plan.Name.ValueString(),
		Description: tfStringToNullable(plan.Description),
		Color:       tfStringToNullable(plan.Color),
	}

	tflog.Debug(ctx, "Creating project tag", map[string]any{
		"project_id": plan.ProjectID.ValueString(),
		"name":       createReq.Name,
	})

	createResp, err := client.Write(r.client.API.PostProjectTagWithResponse(ctx, createReq))
	var created *openapi.ProjectTag
	if err == nil {
		created, err = client.Created(createResp.HTTPResponse, createResp.JSON200, "project tag")
	}
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Creating Braintrust Project Tag",
			"Could not create project tag, unexpected error: "+err.Error(),
		)
		return
	}

	// Re-read to get the full state.
	tag, err := getProjectTag(ctx, r.client, created.Id.String())
	if err != nil {
		mapProjectTagToModel(created, &plan)
		resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
		resp.Diagnostics.AddError(
			"Error Reading Braintrust Project Tag After Create",
			"Tag was created (ID: "+created.Id.String()+") but could not be read: "+err.Error(),
		)
		return
	}

	mapProjectTagToModel(tag, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *projectTagResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state projectTagModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !requireClient(r.client, &resp.Diagnostics) {
		return
	}

	tag, err := getProjectTag(ctx, r.client, state.ID.ValueString())
	if err != nil {
		if client.IsNotFoundError(err) {
			tflog.Warn(ctx, "Project tag not found, removing from state", map[string]any{
				"id": state.ID.ValueString(),
			})
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Error Reading Braintrust Project Tag",
			"Could not read project tag ID "+state.ID.ValueString()+": "+err.Error(),
		)
		return
	}

	mapProjectTagToModel(tag, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *projectTagResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan projectTagModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var state projectTagModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !requireClient(r.client, &resp.Diagnostics) {
		return
	}

	updateReq := openapi.PatchProjectTag{
		Name:        nullable.NewNullableWithValue(plan.Name.ValueString()),
		Description: patchString(plan.Description, state.Description),
		Color:       patchString(plan.Color, state.Color),
	}

	updateResp, err := client.Write(r.client.API.PatchProjectTagIdWithResponse(ctx, state.ID.ValueString(), updateReq))
	var tag *openapi.ProjectTag
	if err == nil {
		tag, err = client.Written(updateResp.JSON200)
	}
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Updating Braintrust Project Tag",
			"Could not update project tag ID "+state.ID.ValueString()+": "+err.Error(),
		)
		return
	}

	mapProjectTagToModel(tag, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *projectTagResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state projectTagModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !requireClient(r.client, &resp.Diagnostics) {
		return
	}

	_, err := client.Write(r.client.API.DeleteProjectTagIdWithResponse(ctx, state.ID.ValueString()))
	if err != nil {
		if client.IsNotFoundError(err) {
			return
		}
		resp.Diagnostics.AddError(
			"Error Deleting Braintrust Project Tag",
			"Could not delete project tag ID "+state.ID.ValueString()+": "+err.Error(),
		)
	}
}

func getProjectTag(ctx context.Context, c *client.Client, id string) (*openapi.ProjectTag, error) {
	resp, err := c.API.GetProjectTagIdWithResponse(ctx, id)
	if err != nil {
		return nil, err
	}
	return client.JSON(resp.JSON200)
}

func (r *projectTagResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
