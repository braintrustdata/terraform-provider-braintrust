package hcl

import (
	"strings"
	"testing"

	"github.com/braintrustdata/terraform-provider-braintrust/internal/terraforming/schema"
	"github.com/hashicorp/hcl/v2/hclparse"
)

func TestWriterEmitsNestedConfigAndUnlistedFields(t *testing.T) {
	writer := NewWriter(schema.SchemaRegistry{
		"braintrust_project_automation": &schema.ResourceSchema{},
	})
	got := writer.WriteResources([]ResourceBlock{{
		Type: "braintrust_project_automation", Name: "test",
		Fields: map[string]any{
			"name": "test",
			"retention_config": map[string]any{
				"retention_days": float64(90),
				"nested":         map[string]any{"enabled": true, "types": []any{"logs", "scores"}},
			},
		},
	}})
	parser := hclparse.NewParser()
	_, diags := parser.ParseHCL(got, "generated.tf")
	if diags.HasErrors() {
		t.Fatalf("generated HCL is invalid: %s", diags.Error())
	}
	for _, want := range []string{"retention_config", "retention_days", "nested", "enabled", "types"} {
		if !strings.Contains(string(got), want) {
			t.Fatalf("generated HCL dropped %q: %s", want, got)
		}
	}
}

func TestWriterVariableTypeIsExpression(t *testing.T) {
	got := string(NewWriter(nil).WriteVariables([]VariableBlock{{Name: "secret"}}))
	if !strings.Contains(got, "type      = string") && !strings.Contains(got, "type = string") {
		t.Fatalf("expected Terraform type expression, got %s", got)
	}
	if strings.Contains(got, `type = "string"`) {
		t.Fatalf("type constraint is quoted: %s", got)
	}
}

func TestWriterCommentsEveryLineOfNames(t *testing.T) {
	got := string(NewWriter(nil).WriteResources([]ResourceBlock{{
		Type: "braintrust_project", Name: "test", Comment: "normal\nresource \"evil\" \"injected\" {}",
		Fields: map[string]any{"name": "normal"},
	}}))
	if !strings.Contains(got, "# resource \"evil\" \"injected\" {}") {
		t.Fatalf("newline in name escaped the comment: %s", got)
	}
	parser := hclparse.NewParser()
	_, diags := parser.ParseHCL([]byte(got), "generated.tf")
	if diags.HasErrors() {
		t.Fatalf("generated HCL is invalid: %s", diags.Error())
	}
}

func TestWriterFormatsBlocksAndJSON(t *testing.T) {
	writer := NewWriter(nil)
	got := string(writer.WriteResources([]ResourceBlock{{
		Type: "braintrust_view", Name: "testing", Comment: "Testing",
		Fields: map[string]any{
			"name":           "Testing",
			"options_json":   `{"options":{"rangeValue":"30d","projectId":""},"viewType":"monitor"}`,
			"view_data_json": `{"search":null,"custom_charts":{"label":"${literal}","count":2}}`,
		},
	}}))
	for _, want := range []string{
		"# Testing\nresource \"braintrust_view\" \"testing\" {",
		"  name = \"Testing\"",
		"options_json = jsonencode({",
		"view_data_json = jsonencode({",
		"search = null",
		"$${literal}",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("generated HCL is missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, `\\"options\\"`) {
		t.Fatalf("JSON was emitted as an escaped string:\n%s", got)
	}
	parser := hclparse.NewParser()
	_, diags := parser.ParseHCL([]byte(got), "generated.tf")
	if diags.HasErrors() {
		t.Fatalf("generated HCL is invalid: %s", diags.Error())
	}

	variables := string(writer.WriteVariables([]VariableBlock{{Name: "secret", Description: "Secret"}}))
	if strings.HasPrefix(variables, " ") || !strings.Contains(variables, "  sensitive   = true") {
		t.Fatalf("variable block is not formatted:\n%s", variables)
	}
}
