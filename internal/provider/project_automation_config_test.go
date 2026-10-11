package provider

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestBuildExportConfigRequiresCredentialsAndQuery(t *testing.T) {
	config := &exportConfigModel{
		ExportType:      types.StringValue("log_spans"),
		ExportPath:      types.StringValue("s3://test-bucket/prefix"),
		Format:          types.StringValue("jsonl"),
		IntervalSeconds: types.Int64Value(3600),
	}
	if _, err := buildExportConfigJSON(config); err == nil || !strings.Contains(err.Error(), "aws_iam_credentials") {
		t.Fatalf("expected missing credentials error, got %v", err)
	}

	config.AWSIAMCredentials = &awsIAMCredentialsModel{
		RoleARN:    types.StringValue("arn:aws:iam::000000000000:role/test"),
		ExternalID: types.StringNull(),
	}
	if _, err := buildExportConfigJSON(config); err == nil || !strings.Contains(err.Error(), "external_id") {
		t.Fatalf("expected missing external ID error, got %v", err)
	}

	config.AWSIAMCredentials.ExternalID = types.StringValue("test-external-id")
	config.ExportType = types.StringValue("btql_query")
	if _, err := buildExportConfigJSON(config); err == nil || !strings.Contains(err.Error(), "btql_query") {
		t.Fatalf("expected missing BTQL query error, got %v", err)
	}

	config.ExportType = types.StringValue("log_spans")
	payload, err := buildExportConfigJSON(config)
	if err != nil {
		t.Fatal(err)
	}
	var decoded struct {
		Credentials struct {
			Type       string `json:"type"`
			RoleARN    string `json:"role_arn"`
			ExternalID string `json:"external_id"`
		} `json:"credentials"`
	}
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Credentials.Type != "aws_iam" || decoded.Credentials.RoleARN != "arn:aws:iam::000000000000:role/test" || decoded.Credentials.ExternalID != "test-external-id" {
		t.Fatalf("unexpected credentials payload: %+v", decoded.Credentials)
	}
}

func TestBuildAutomationConfigRejectsAmbiguousVariants(t *testing.T) {
	model := &projectAutomationModel{}
	if _, err := buildConfigJSON(model); err == nil || !strings.Contains(err.Error(), "exactly one") {
		t.Fatalf("expected missing config error, got %v", err)
	}
	model.RetentionConfig = &retentionConfigModel{ObjectType: types.StringValue("project_logs"), RetentionDays: types.Int64Value(90)}
	model.ExportConfig = &exportConfigModel{}
	if _, err := buildConfigJSON(model); err == nil || !strings.Contains(err.Error(), "exactly one") {
		t.Fatalf("expected multiple config error, got %v", err)
	}
	if _, err := buildActionJSON(&webhookActionModel{URL: types.StringValue("https://example.invalid")}, &slackActionModel{}); err == nil || !strings.Contains(err.Error(), "exactly one") {
		t.Fatalf("expected multiple action error, got %v", err)
	}
}

func TestRawAutomationConfigRoundTrip(t *testing.T) {
	configured := types.StringValue(`{ "event_type": "topic", "sampling_rate": 0.5, "facet_functions": [], "topic_map_functions": [] }`)
	model := &projectAutomationModel{ConfigJSON: configured}
	payload, err := buildConfigJSON(model)
	if err != nil {
		t.Fatal(err)
	}
	if err := parseConfigToModel(payload, model); err != nil {
		t.Fatal(err)
	}
	if model.ConfigJSON != configured || model.LogsConfig != nil || model.ExportConfig != nil || model.RetentionConfig != nil || model.EnvironmentUpdateConfig != nil {
		t.Fatalf("raw config was not preserved: %#v", model)
	}

	imported := &projectAutomationModel{ConfigJSON: types.StringNull()}
	if err := parseConfigToModel(payload, imported); err != nil {
		t.Fatal(err)
	}
	if imported.ConfigJSON.IsNull() || imported.ConfigJSON.IsUnknown() {
		t.Fatal("unknown automation config was lost on import")
	}
	var got, want any
	if err := json.Unmarshal([]byte(imported.ConfigJSON.ValueString()), &got); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(payload, &want); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("imported config = %v, want %v", got, want)
	}
}

func TestRawAutomationConfigValidation(t *testing.T) {
	for _, raw := range []string{`[]`, `{}`, `{"event_type":42}`, `{"event_type":""}`, `not-json`} {
		model := &projectAutomationModel{ConfigJSON: types.StringValue(raw)}
		if _, err := buildConfigJSON(model); err == nil {
			t.Fatalf("accepted invalid config_json %q", raw)
		}
	}
	model := &projectAutomationModel{
		ConfigJSON:      types.StringValue(`{"event_type":"topic"}`),
		RetentionConfig: &retentionConfigModel{ObjectType: types.StringValue("project_logs"), RetentionDays: types.Int64Value(90)},
	}
	if _, err := buildConfigJSON(model); err == nil {
		t.Fatal("accepted both raw and structured automation config")
	}
}
