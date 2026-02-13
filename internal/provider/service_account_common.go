package provider

import (
	"github.com/braintrustdata/terraform-provider-braintrust/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// serviceAccountModel is the Terraform data model for the service account resource.
type serviceAccountModel struct {
	ID      types.String `tfsdk:"id"`
	Name    types.String `tfsdk:"name"`
	Email   types.String `tfsdk:"email"`
	Created types.String `tfsdk:"created"`
}

func mapUserToServiceAccountModel(user *client.User, model *serviceAccountModel) {
	model.ID = types.StringValue(user.ID)
	model.Email = stringPtrToTFValue(user.Email)
	model.Created = stringPtrToTFValue(user.Created)
	// Name is intentionally preserved from configuration/state. The user object
	// does not expose a stable service-account display name field.
}
