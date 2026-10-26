package provider

import (
	"context"
	"encoding/json"

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
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/oapi-codegen/nullable"
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
			"description":   optionalComputedStringAttribute("Textual description of the dataset."),
			"tags":          schema.ListAttribute{Optional: true, Computed: true, ElementType: types.StringType, Description: "Dataset tags. Use an empty list to clear all tags."},
			"metadata_json": optionalComputedStringAttribute("JSON-encoded dataset metadata object. Use the JSON literal null to clear it."),
			"url_slug":      computedStringAttribute("URL slug assigned to the dataset by Braintrust."),
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

	projectID, err := parseUUID(plan.ProjectID, "project_id")
	if err != nil {
		resp.Diagnostics.AddAttributeError(path.Root("project_id"), "Invalid Project ID", err.Error())
		return
	}
	createReq := openapi.CreateDataset{
		ProjectId:   projectID,
		Name:        plan.Name.ValueString(),
		Description: tfStringToNullable(plan.Description),
		Tags:        nullableSlice(stringListFromModel(plan.Tags)),
	}
	metadata, err := datasetMetadataInput(plan.MetadataJSON)
	if err != nil {
		resp.Diagnostics.AddError("Invalid Dataset Metadata", err.Error())
		return
	}
	body, err := client.JSONBody(createReq, map[string]json.RawMessage{"metadata": metadata})
	if err != nil {
		resp.Diagnostics.AddError("Error Creating Braintrust Dataset", err.Error())
		return
	}

	tflog.Debug(ctx, "Creating dataset", map[string]any{
		"project_id": plan.ProjectID.ValueString(),
		"name":       createReq.Name,
	})

	createResp, err := client.Write(r.client.API.PostDatasetWithBodyWithResponse(ctx, client.JSONContentType, body))
	var created *dataset
	if err == nil {
		var createdBody *openapi.Dataset
		createdBody, err = client.Created(createResp.HTTPResponse, createResp.JSON200, "dataset")
		if err == nil {
			created, err = client.WrittenRaw(createdBody, createResp.Body)
		}
	}
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Creating Braintrust Dataset",
			"Could not create dataset, unexpected error: "+err.Error(),
		)
		return
	}

	// Re-read to get the full state -- create responses may omit some fields.
	dataset, err := getDataset(ctx, r.client, created.Value.Id.String())
	if err != nil {
		mapDatasetToModel(created, &plan)
		resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
		resp.Diagnostics.AddError(
			"Error Reading Braintrust Dataset After Create",
			"Dataset was created (ID: "+created.Value.Id.String()+") but could not be read: "+err.Error(),
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

	dataset, err := getDataset(ctx, r.client, state.ID.ValueString())
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

	updateReq := openapi.PatchDataset{
		Name:        nullable.NewNullableWithValue(plan.Name.ValueString()),
		Description: patchString(plan.Description, state.Description),
	}
	if !plan.Tags.IsNull() && !plan.Tags.IsUnknown() && !plan.Tags.Equal(state.Tags) {
		tags := stringListFromModel(plan.Tags)
		if tags == nil {
			tags = []string{}
		}
		updateReq.Tags = nullable.NewNullableWithValue(tags)
	}
	metadata, err := datasetMetadataInput(plan.MetadataJSON)
	if err != nil {
		resp.Diagnostics.AddError("Invalid Dataset Metadata", err.Error())
		return
	}
	raw := map[string]json.RawMessage{}
	if metadata != nil && !plan.MetadataJSON.Equal(state.MetadataJSON) {
		raw["metadata"] = metadata
	}
	body, err := client.JSONBody(updateReq, raw)
	if err != nil {
		resp.Diagnostics.AddError("Error Updating Braintrust Dataset", err.Error())
		return
	}

	updateResp, err := client.Write(r.client.API.PatchDatasetIdWithBodyWithResponse(ctx, state.ID.ValueString(), client.JSONContentType, body))
	var dataset *dataset
	if err == nil {
		dataset, err = client.WrittenRaw(updateResp.JSON200, updateResp.Body)
	}
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

	_, err := client.Write(r.client.API.DeleteDatasetIdWithResponse(ctx, state.ID.ValueString()))
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
