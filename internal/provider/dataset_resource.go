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
	_ resource.Resource                = &datasetResource{}
	_ resource.ResourceWithConfigure   = &datasetResource{}
	_ resource.ResourceWithImportState = &datasetResource{}
)

// NewDatasetResource returns a new dataset resource.
func NewDatasetResource() resource.Resource {
	return &datasetResource{}
}

type datasetResource struct {
	client *client.Client
}

func (r *datasetResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_dataset"
}

func (r *datasetResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Version:             0,
		Description:         "Manages a Braintrust dataset.",
		MarkdownDescription: "Manages a Braintrust [dataset](https://www.braintrustdata.com/docs/guides/datasets). Datasets belong to a project and store evaluation data.",
		Attributes: map[string]schema.Attribute{
			"id":         computedStringAttribute("Unique identifier for the dataset (UUID)."),
			"created":    computedStringAttribute("Date of dataset creation."),
			"deleted_at": computedStringAttribute("Date of dataset deletion, or null if the dataset is still active."),
			"user_id":    computedStringAttribute("Identifies the user who created the dataset."),
			"project_id": schema.StringAttribute{
				Required:    true,
				Description: "Unique identifier for the project that the dataset belongs to.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"name": schema.StringAttribute{
				Required:    true,
				Description: "Name of the dataset. Within a project, dataset names are unique.",
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"description": optionalComputedStringAttribute("Textual description of the dataset."),
		},
	}
}

func (r *datasetResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = clientFromProviderData(req.ProviderData, &resp.Diagnostics)
}

func (r *datasetResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan datasetModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !requireClient(r.client, &resp.Diagnostics) {
		return
	}

	createReq := client.CreateDatasetRequest{
		ProjectID:   plan.ProjectID.ValueString(),
		Name:        plan.Name.ValueString(),
		Description: tfStringToPtr(plan.Description),
	}

	tflog.Debug(ctx, "Creating dataset", map[string]any{
		"project_id": createReq.ProjectID,
		"name":       createReq.Name,
	})

	created, err := r.client.CreateDataset(ctx, createReq)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Creating Braintrust Dataset",
			"Could not create dataset, unexpected error: "+err.Error(),
		)
		return
	}

	// Re-read to get the full state -- create responses may omit some fields.
	dataset, err := r.client.GetDataset(ctx, created.ID)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Reading Braintrust Dataset After Create",
			"Dataset was created (ID: "+created.ID+") but could not be read: "+err.Error(),
		)
		return
	}

	mapDatasetToModel(dataset, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *datasetResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state datasetModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !requireClient(r.client, &resp.Diagnostics) {
		return
	}

	dataset, err := r.client.GetDataset(ctx, state.ID.ValueString())
	if err != nil {
		if client.IsNotFoundError(err) {
			tflog.Warn(ctx, "Dataset not found, removing from state", map[string]any{
				"id": state.ID.ValueString(),
			})
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Error Reading Braintrust Dataset",
			"Could not read dataset ID "+state.ID.ValueString()+": "+err.Error(),
		)
		return
	}

	mapDatasetToModel(dataset, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *datasetResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan datasetModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var state datasetModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !requireClient(r.client, &resp.Diagnostics) {
		return
	}

	name := plan.Name.ValueString()
	updateReq := client.UpdateDatasetRequest{
		Name: &name,
	}

	if !plan.Description.IsNull() && !plan.Description.IsUnknown() {
		desc := plan.Description.ValueString()
		updateReq.Description = &desc
	} else if plan.Description.IsNull() && !state.Description.IsNull() {
		empty := ""
		updateReq.Description = &empty
	}

	dataset, err := r.client.UpdateDataset(ctx, state.ID.ValueString(), updateReq)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Updating Braintrust Dataset",
			"Could not update dataset ID "+state.ID.ValueString()+": "+err.Error(),
		)
		return
	}

	mapDatasetToModel(dataset, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *datasetResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state datasetModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !requireClient(r.client, &resp.Diagnostics) {
		return
	}

	err := r.client.DeleteDataset(ctx, state.ID.ValueString())
	if err != nil {
		if client.IsNotFoundError(err) {
			return
		}
		resp.Diagnostics.AddError(
			"Error Deleting Braintrust Dataset",
			"Could not delete dataset ID "+state.ID.ValueString()+": "+err.Error(),
		)
	}
}

func (r *datasetResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
