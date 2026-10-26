// Package schema reads the provider's resource schemas to determine which
// attributes each Braintrust resource type accepts in Terraform configuration.
package schema

import (
	"context"
	"fmt"

	"github.com/braintrustdata/terraform-provider-braintrust/internal/provider"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	tfprovider "github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
)

// FieldType represents the value type of an attribute.
type FieldType string

const (
	FieldTypeString  FieldType = "string"
	FieldTypeNumber  FieldType = "number"
	FieldTypeInteger FieldType = "integer"
	FieldTypeBool    FieldType = "boolean"
	FieldTypeArray   FieldType = "array"
	FieldTypeObject  FieldType = "object"
	FieldTypeUnknown FieldType = "unknown"
)

// Field describes a single configurable attribute of a resource type.
type Field struct {
	Name     string    // Terraform attribute name
	Type     FieldType // Value type
	Required bool      // Is this attribute required in configuration?
}

// ResourceSchema holds the configurable attributes for a Terraform resource type.
type ResourceSchema struct {
	TerraformType string  // e.g. "braintrust_project"
	Fields        []Field // Attributes that may appear in HCL output
}

// SchemaRegistry maps Terraform type names to their schemas.
type SchemaRegistry map[string]*ResourceSchema

// LoadRegistry builds the registry from the provider's resource schemas, so
// generated configuration uses exactly the attributes the provider accepts.
func LoadRegistry() (SchemaRegistry, error) {
	ctx := context.Background()
	p := provider.New("exporter")()
	var meta tfprovider.MetadataResponse
	p.Metadata(ctx, tfprovider.MetadataRequest{}, &meta)

	registry := make(SchemaRegistry)
	for _, newResource := range p.Resources(ctx) {
		r := newResource()
		var name resource.MetadataResponse
		r.Metadata(ctx, resource.MetadataRequest{ProviderTypeName: meta.TypeName}, &name)
		var resp resource.SchemaResponse
		r.Schema(ctx, resource.SchemaRequest{}, &resp)
		if resp.Diagnostics.HasError() {
			return nil, fmt.Errorf("load %s schema: %v", name.TypeName, resp.Diagnostics)
		}

		var fields []Field
		for attrName, a := range resp.Schema.Attributes {
			// Computed-only attributes are set by the provider, never by config.
			if !a.IsRequired() && !a.IsOptional() {
				continue
			}
			fields = append(fields, Field{Name: attrName, Type: fieldType(a.GetType()), Required: a.IsRequired()})
		}
		for blockName, b := range resp.Schema.Blocks {
			fields = append(fields, Field{Name: blockName, Type: fieldType(b.Type())})
		}
		registry[name.TypeName] = &ResourceSchema{TerraformType: name.TypeName, Fields: fields}
	}
	return registry, nil
}

func fieldType(t attr.Type) FieldType {
	switch t.(type) {
	case basetypes.StringTypable:
		return FieldTypeString
	case basetypes.Int64Typable, basetypes.Int32Typable:
		return FieldTypeInteger
	case basetypes.Float64Typable, basetypes.Float32Typable, basetypes.NumberTypable:
		return FieldTypeNumber
	case basetypes.BoolTypable:
		return FieldTypeBool
	case basetypes.ListTypable, basetypes.SetTypable:
		return FieldTypeArray
	case basetypes.ObjectTypable, basetypes.MapTypable:
		return FieldTypeObject
	}
	return FieldTypeUnknown
}

// WritableFields returns the fields for a resource type that should be written
// to Terraform configuration. This is the primary interface used by the HCL writer.
func (r *ResourceSchema) WritableFields() []Field {
	return r.Fields
}
