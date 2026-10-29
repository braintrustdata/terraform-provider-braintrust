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
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/oapi-codegen/nullable"
)

var (
	_ resource.Resource                = &mcpServerResource{}
	_ resource.ResourceWithConfigure   = &mcpServerResource{}
	_ resource.ResourceWithImportState = &mcpServerResource{}
)

func NewMCPServerResource() resource.Resource { return &mcpServerResource{} }

type mcpServerResource struct{ client *client.Client }

type mcpServerModel struct {
	ID          types.String `tfsdk:"id"`
	ProjectID   types.String `tfsdk:"project_id"`
	Name        types.String `tfsdk:"name"`
	URL         types.String `tfsdk:"url"`
	Description types.String `tfsdk:"description"`
	UserID      types.String `tfsdk:"user_id"`
	Created     types.String `tfsdk:"created"`
	DeletedAt   types.String `tfsdk:"deleted_at"`
}

func (r *mcpServerResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_mcp_server"
}

func (r *mcpServerResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a project-scoped MCP server connection in Braintrust.",
		Attributes: map[string]schema.Attribute{
			"id": computedStringAttribute("Unique identifier for the MCP server (UUID)."),
			"project_id": schema.StringAttribute{
				Required: true, Description: "Unique identifier for the project that the MCP server belongs to.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"name": schema.StringAttribute{
				Required: true, Description: "Name of the MCP server, unique within the project.",
				Validators: []validator.String{stringvalidator.LengthAtLeast(1)},
			},
			"url": schema.StringAttribute{
				Required: true, Description: "URL of the MCP server endpoint.",
				Validators: []validator.String{stringvalidator.LengthAtLeast(1)},
			},
			"description": schema.StringAttribute{Optional: true, Description: "Textual description of the MCP server."},
			"user_id":     computedStringAttribute("Identifies the user who created the MCP server."),
			"created":     computedStringAttribute("Date of MCP server creation."),
			"deleted_at":  computedStringAttribute("Date of MCP server deletion, or null if the MCP server is still active."),
		},
	}
}

func (r *mcpServerResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = clientFromProviderData(req.ProviderData, &resp.Diagnostics)
}

func mapMCPServerToModel(server *openapi.MCPServer, model *mcpServerModel) {
	model.ID = types.StringValue(server.Id.String())
	model.ProjectID = types.StringValue(server.ProjectId.String())
	model.Name = types.StringValue(server.Name)
	model.URL = types.StringValue(server.Url)
	model.Description = nullableStringToTF(server.Description)
	model.UserID = nullableUUIDToTF(server.UserId)
	model.Created = nullableTimeToTF(server.Created, model.Created)
	model.DeletedAt = nullableTimeToTF(server.DeletedAt, model.DeletedAt)
}

func getMCPServer(ctx context.Context, c *client.Client, id string) (*openapi.MCPServer, error) {
	resp, err := c.API.GetMcpServerIdWithResponse(ctx, id)
	if err != nil {
		return nil, err
	}
	return client.JSON(resp.JSON200)
}

func (r *mcpServerResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan mcpServerModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() || !requireClient(r.client, &resp.Diagnostics) {
		return
	}
	projectID, err := parseUUID(plan.ProjectID, "project_id")
	if err != nil {
		resp.Diagnostics.AddAttributeError(path.Root("project_id"), "Invalid Project ID", err.Error())
		return
	}
	createResp, err := client.Write(r.client.API.PostMcpServerWithResponse(ctx, openapi.CreateMCPServer{
		ProjectId: projectID, Name: plan.Name.ValueString(),
		Url: plan.URL.ValueString(), Description: tfStringToNullable(plan.Description),
	}))
	var created *openapi.MCPServer
	if err == nil {
		created, err = client.Created(createResp.HTTPResponse, createResp.JSON200, "MCP server")
	}
	if err != nil {
		resp.Diagnostics.AddError("Error Creating Braintrust MCP Server", err.Error())
		return
	}
	server, err := getMCPServer(ctx, r.client, created.Id.String())
	if err != nil {
		mapMCPServerToModel(created, &plan)
		resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
		resp.Diagnostics.AddError("Error Reading MCP Server After Create", "Created ID "+created.Id.String()+": "+err.Error())
		return
	}
	mapMCPServerToModel(server, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *mcpServerResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state mcpServerModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() || !requireClient(r.client, &resp.Diagnostics) {
		return
	}
	server, err := getMCPServer(ctx, r.client, state.ID.ValueString())
	if err != nil {
		if client.IsNotFoundError(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error Reading Braintrust MCP Server", err.Error())
		return
	}
	mapMCPServerToModel(server, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *mcpServerResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state mcpServerModel
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
	updateResp, err := client.Write(r.client.API.PatchMcpServerIdWithResponse(ctx, state.ID.ValueString(), openapi.PatchMCPServer{
		Name: nullable.NewNullableWithValue(plan.Name.ValueString()), Url: nullable.NewNullableWithValue(plan.URL.ValueString()), Description: description,
	}))
	var server *openapi.MCPServer
	if err == nil {
		server, err = client.Written(updateResp.JSON200)
	}
	if err != nil {
		resp.Diagnostics.AddError("Error Updating Braintrust MCP Server", err.Error())
		return
	}
	mapMCPServerToModel(server, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *mcpServerResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state mcpServerModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() || !requireClient(r.client, &resp.Diagnostics) {
		return
	}
	if _, err := client.Write(r.client.API.DeleteMcpServerIdWithResponse(ctx, state.ID.ValueString())); err != nil && !client.IsNotFoundError(err) {
		resp.Diagnostics.AddError("Error Deleting Braintrust MCP Server", err.Error())
	}
}

func (r *mcpServerResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
