package provider

import (
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// serviceAccountModel is the Terraform data model for the service account resource.
type serviceAccountModel struct {
	ID      types.String `tfsdk:"id"`
	Name    types.String `tfsdk:"name"`
	Email   types.String `tfsdk:"email"`
	Created types.String `tfsdk:"created"`
}
