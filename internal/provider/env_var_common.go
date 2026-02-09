package provider

import (
	"github.com/braintrustdata/terraform-provider-braintrust/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// envVarModel is the Terraform data model for the env var resource.
type envVarModel struct {
	ID             types.String `tfsdk:"id"`
	ObjectType     types.String `tfsdk:"object_type"`
	ObjectID       types.String `tfsdk:"object_id"`
	Name           types.String `tfsdk:"name"`
	Value          types.String `tfsdk:"value"`
	Created        types.String `tfsdk:"created"`
	Used           types.String `tfsdk:"used"`
	SecretType     types.String `tfsdk:"secret_type"`
	SecretCategory types.String `tfsdk:"secret_category"`
}

// mapEnvVarToModel maps a client.EnvVar to the Terraform model.
// Note: value is write-only and never returned by the API, so we
// leave model.Value unchanged (preserving whatever Terraform has in state).
func mapEnvVarToModel(envVar *client.EnvVar, model *envVarModel) {
	model.ID = types.StringValue(envVar.ID)
	model.ObjectType = types.StringValue(envVar.ObjectType)
	model.ObjectID = types.StringValue(envVar.ObjectID)
	model.Name = types.StringValue(envVar.Name)
	model.Created = stringPtrToTFValue(envVar.Created)
	model.Used = stringPtrToTFValue(envVar.Used)
	model.SecretType = stringPtrToTFValue(envVar.SecretType)
	model.SecretCategory = stringPtrToTFValue(envVar.SecretCategory)
	// model.Value is intentionally NOT set here -- see doc comment.
}
