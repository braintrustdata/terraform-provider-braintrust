package provider

import (
	"github.com/braintrustdata/terraform-provider-braintrust/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// datasetModel is the shared data model for both the dataset resource and data source.
type datasetModel struct {
	ID          types.String `tfsdk:"id"`
	ProjectID   types.String `tfsdk:"project_id"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
	Created     types.String `tfsdk:"created"`
	DeletedAt   types.String `tfsdk:"deleted_at"`
	UserID      types.String `tfsdk:"user_id"`
}

// mapDatasetToModel maps a client.Dataset to the shared Terraform model.
func mapDatasetToModel(dataset *client.Dataset, model *datasetModel) {
	model.ID = types.StringValue(dataset.ID)
	model.ProjectID = types.StringValue(dataset.ProjectID)
	model.Name = types.StringValue(dataset.Name)
	model.Description = stringPtrToTFValue(dataset.Description)
	model.Created = stringPtrToTFValue(dataset.Created)
	model.DeletedAt = stringPtrToTFValue(dataset.DeletedAt)
	model.UserID = stringPtrToTFValue(dataset.UserID)
}
