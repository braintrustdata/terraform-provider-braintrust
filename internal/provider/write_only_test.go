package provider

import (
	"context"
	"encoding/json"
	"maps"
	"strings"
	"testing"

	"github.com/braintrustdata/braintrust-sdk-go/openapi"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

func TestWriteOnlyMappingsDiscardPlaintext(t *testing.T) {
	secret := aiSecretModel{
		Secret: types.StringValue("old-plaintext"), SecretVersion: types.Int64Value(3),
	}
	mapAISecretToModel(&openapi.AISecret{}, &secret)
	if !secret.Secret.IsNull() || !secret.SecretVersion.Equal(types.Int64Value(3)) {
		t.Fatal("AI secret mapping retained plaintext or lost the version")
	}
	envVar := envVarModel{
		Value: types.StringValue("old-plaintext"), ValueVersion: types.Int64Value(4),
	}
	mapEnvVarToModel(&openapi.EnvVar{}, &envVar)
	if !envVar.Value.IsNull() || !envVar.ValueVersion.Equal(types.Int64Value(4)) {
		t.Fatal("environment variable mapping retained plaintext or lost the version")
	}
}

func TestWriteOnlyValidation(t *testing.T) {
	ctx := context.Background()
	const plaintext = "validation-must-not-disclose-this"
	for _, resourceCase := range []struct {
		name     string
		field    writeOnlyField
		resource resource.Resource
	}{
		{"ai_secret", aiSecretWriteOnly, NewAISecretResource()},
		{"env_var", envVarWriteOnly, NewEnvVarResource()},
	} {
		t.Run(resourceCase.name, func(t *testing.T) {
			field := resourceCase.field
			var schemaResp resource.SchemaResponse
			resourceCase.resource.Schema(ctx, resource.SchemaRequest{}, &schemaResp)
			valueAttr := schemaResp.Schema.Attributes[field.valueName].(schema.StringAttribute)
			versionAttr := schemaResp.Schema.Attributes[field.versionName].(schema.Int64Attribute)
			for _, tc := range []struct {
				name        string
				value       types.String
				version     types.Int64
				configError bool
				applyError  bool
			}{
				{"adoption", types.StringNull(), types.Int64Null(), false, false},
				{"configured", types.StringValue(plaintext), types.Int64Value(1), false, false},
				{"empty value still needs version", types.StringValue(""), types.Int64Null(), true, true},
				{"missing version", types.StringValue(plaintext), types.Int64Null(), true, true},
				{"zero version", types.StringValue(plaintext), types.Int64Value(0), true, true},
				{"negative version", types.StringValue(plaintext), types.Int64Value(-1), true, true},
				{"null value with version", types.StringNull(), types.Int64Value(2), !field.allowClear, !field.allowClear},
				{"unknown value", types.StringUnknown(), types.Int64Value(1), false, true},
				{"unknown value missing version", types.StringUnknown(), types.Int64Null(), false, true},
				{"unknown version", types.StringValue(plaintext), types.Int64Unknown(), false, true},
				{"both unknown", types.StringUnknown(), types.Int64Unknown(), false, true},
			} {
				t.Run(tc.name, func(t *testing.T) {
					value, err := tc.value.ToTerraformValue(ctx)
					if err != nil {
						t.Fatal(err)
					}
					version, err := tc.version.ToTerraformValue(ctx)
					if err != nil {
						t.Fatal(err)
					}
					config := tfsdk.Config{
						Schema: schemaResp.Schema,
						Raw: writeOnlyTestRaw(ctx, schemaResp.Schema, map[string]tftypes.Value{
							field.valueName: value, field.versionName: version,
						}),
					}
					var stringResp validator.StringResponse
					for _, v := range valueAttr.Validators {
						v.ValidateString(ctx, validator.StringRequest{
							Path: path.Root(field.valueName), PathExpression: path.MatchRoot(field.valueName),
							Config: config, ConfigValue: tc.value,
						}, &stringResp)
					}
					var intResp validator.Int64Response
					for _, v := range versionAttr.Validators {
						v.ValidateInt64(ctx, validator.Int64Request{
							Path: path.Root(field.versionName), PathExpression: path.MatchRoot(field.versionName),
							Config: config, ConfigValue: tc.version,
						}, &intResp)
					}
					diagnostics := stringResp.Diagnostics
					diagnostics.Append(intResp.Diagnostics...)
					if diagnostics.HasError() != tc.configError {
						t.Fatalf("config errors = %v, want %v: %v", diagnostics.HasError(), tc.configError, diagnostics)
					}
					var applyDiagnostics diag.Diagnostics
					field.validate(tc.value, tc.version, &applyDiagnostics)
					if applyDiagnostics.HasError() != tc.applyError {
						t.Fatalf("apply errors = %v, want %v: %v", applyDiagnostics.HasError(), tc.applyError, applyDiagnostics)
					}
					diagnostics.Append(applyDiagnostics...)
					for _, d := range diagnostics {
						if strings.Contains(d.Summary()+d.Detail(), plaintext) {
							t.Fatal("validation diagnostic disclosed plaintext")
						}
					}
				})
			}
		})
	}
}

func TestWriteOnlyStateUpgradePreservesMetadata(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name, valueName, versionName string
		resource                     resource.ResourceWithUpgradeState
	}{
		{"ai_secret", "secret", "secret_version", &aiSecretResource{}},
		{"env_var", "value", "value_version", &envVarResource{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var schemaResp resource.SchemaResponse
			tc.resource.Schema(ctx, resource.SchemaRequest{}, &schemaResp)
			prior := make(map[string]any)
			for name := range schemaResp.Schema.Attributes {
				if name != tc.versionName {
					prior[name] = "preserve-" + name
				}
			}
			prior[tc.valueName] = "old-plaintext"
			prior["created"] = nil
			legacy := maps.Clone(prior)
			legacy["attribute_removed_from_schema"] = "ignored"
			raw, err := json.Marshal(legacy)
			if err != nil {
				t.Fatal(err)
			}
			resp := resource.UpgradeStateResponse{State: tfsdk.State{Schema: schemaResp.Schema}}
			tc.resource.UpgradeState(ctx)[0].StateUpgrader(ctx, resource.UpgradeStateRequest{
				RawState: &tfprotov6.RawState{JSON: raw},
			}, &resp)
			if resp.Diagnostics.HasError() {
				t.Fatal(resp.Diagnostics)
			}
			var attributes map[string]tftypes.Value
			if err := resp.State.Raw.As(&attributes); err != nil {
				t.Fatal(err)
			}
			if !attributes[tc.valueName].IsNull() || !attributes[tc.versionName].IsNull() {
				t.Fatal("upgraded state retained plaintext or invented a version")
			}
			for name, value := range prior {
				if name == tc.valueName {
					continue
				}
				if !attributes[name].Equal(tftypes.NewValue(tftypes.String, value)) {
					t.Errorf("metadata attribute %s changed during upgrade", name)
				}
			}

			// A malformed old value must not leak into upgrade diagnostics.
			prior[tc.valueName] = map[string]string{"unexpected": "old-plaintext"}
			raw, err = json.Marshal(prior)
			if err != nil {
				t.Fatal(err)
			}
			resp = resource.UpgradeStateResponse{State: tfsdk.State{Schema: schemaResp.Schema}}
			tc.resource.UpgradeState(ctx)[0].StateUpgrader(ctx, resource.UpgradeStateRequest{
				RawState: &tfprotov6.RawState{JSON: raw},
			}, &resp)
			if !resp.Diagnostics.HasError() {
				t.Fatal("malformed state did not produce an error")
			}
			for _, d := range resp.Diagnostics {
				if strings.Contains(d.Summary()+d.Detail(), "old-plaintext") {
					t.Fatal("upgrade diagnostic disclosed plaintext")
				}
			}
		})
	}
}

func writeOnlyTestRaw(ctx context.Context, resourceSchema schema.Schema, overrides map[string]tftypes.Value) tftypes.Value {
	objectType := resourceSchema.Type().TerraformType(ctx).(tftypes.Object)
	attributes := make(map[string]tftypes.Value, len(objectType.AttributeTypes))
	for name, attrType := range objectType.AttributeTypes {
		attributes[name] = tftypes.NewValue(attrType, nil)
	}
	for name, value := range overrides {
		attributes[name] = value
	}
	return tftypes.NewValue(objectType, attributes)
}
