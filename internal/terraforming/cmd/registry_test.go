package cmd

import (
	"testing"

	"github.com/braintrustdata/terraform-provider-braintrust/internal/terraforming/schema"
)

func TestRegistryCoversEveryExportedType(t *testing.T) {
	registry, err := schema.LoadRegistry()
	if err != nil {
		t.Fatal(err)
	}
	for _, resourceType := range AllResourceTypes {
		if registry[resourceType] == nil {
			t.Errorf("provider has no schema for exported type %s", resourceType)
		}
	}
	fields := map[string]schema.Field{}
	for _, f := range registry["braintrust_project"].Fields {
		fields[f.Name] = f
	}
	if !fields["name"].Required {
		t.Errorf("project name should be required: %+v", fields)
	}
	if _, ok := fields["id"]; ok {
		t.Error("computed-only id must not be writable")
	}
}
