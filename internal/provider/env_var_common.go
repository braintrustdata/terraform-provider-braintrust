package provider

import (
	"context"

	"github.com/braintrustdata/braintrust-sdk-go/openapi"
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

// mapEnvVarToModel maps an SDK env var to the Terraform model.
// Note: value is write-only and never returned by the API, so we
// leave model.Value unchanged (preserving whatever Terraform has in state).
func mapEnvVarToModel(envVar *openapi.EnvVar, model *envVarModel) {
	model.ID = types.StringValue(envVar.Id.String())
	model.ObjectType = types.StringValue(string(envVar.ObjectType))
	model.ObjectID = types.StringValue(envVar.ObjectId.String())
	model.Name = types.StringValue(envVar.Name)
	model.Created = nullableTimeToTF(envVar.Created, model.Created)
	model.Used = nullableTimeToTF(envVar.Used, model.Used)
	model.SecretType = nullableStringToTF(envVar.SecretType)
	if envVar.SecretCategory != nil {
		model.SecretCategory = types.StringValue(string(*envVar.SecretCategory))
	} else {
		model.SecretCategory = types.StringNull()
	}
	// model.Value is intentionally NOT set here -- see doc comment.
}

func getEnvVar(ctx context.Context, c *client.Client, id string) (*openapi.EnvVar, error) {
	resp, err := c.API.GetEnvVarIdWithResponse(ctx, id)
	if err != nil {
		return nil, err
	}
	return client.JSON(resp.JSON200)
}
