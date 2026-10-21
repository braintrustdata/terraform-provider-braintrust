// Package schema parses the embedded OpenAPI spec to determine which fields
// each Braintrust resource type exposes, and which of those should be included
// in generated Terraform configuration.
package schema

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"
)

//go:embed spec.json
var embeddedSpec []byte

// FieldType represents the JSON schema type of a field.
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

// Field describes a single attribute of a resource type.
type Field struct {
	Name     string    // JSON / Terraform attribute name
	Type     FieldType // Primitive type
	Required bool      // Is this field required for creation?
	Nullable bool      // Can this be null?
	ItemType FieldType // For arrays, the type of the items
}

// ResourceSchema holds the writable fields for a Terraform resource type.
type ResourceSchema struct {
	TerraformType string  // e.g. "braintrust_project"
	APIPath       string  // e.g. "/v1/project"
	Fields        []Field // Fields to include in HCL output
}

// SchemaRegistry maps Terraform type names to their schemas.
type SchemaRegistry map[string]*ResourceSchema

// LoadRegistry parses the embedded OpenAPI spec and builds the registry.
func LoadRegistry() (SchemaRegistry, error) {
	return loadRegistryFromBytes(embeddedSpec)
}

// LoadRegistryFromBytes parses a custom spec (e.g., freshly downloaded).
func LoadRegistryFromBytes(data []byte) (SchemaRegistry, error) {
	return loadRegistryFromBytes(data)
}

// openAPISpec is a minimal representation of the parts of the OpenAPI spec we need.
type openAPISpec struct {
	Components struct {
		Schemas map[string]openAPISchema `json:"schemas"`
	} `json:"components"`
}

type openAPISchema struct {
	Properties map[string]openAPIProperty `json:"properties"`
	Required   []string                   `json:"required"`
}

type openAPIProperty struct {
	Type    string         `json:"type"`
	Enum    []any          `json:"enum"`
	Items   *openAPIItems  `json:"items"`
	Ref     string         `json:"$ref"`
	AnyOf   []openAPIAnyOf `json:"anyOf"`
	OneOf   []openAPIAnyOf `json:"oneOf"`
	Default any            `json:"default"`
}

type openAPIItems struct {
	Type string `json:"type"`
	Ref  string `json:"$ref"`
}

type openAPIAnyOf struct {
	Type string `json:"type"`
	Ref  string `json:"$ref"`
}

func loadRegistryFromBytes(data []byte) (SchemaRegistry, error) {
	var spec openAPISpec
	if err := json.Unmarshal(data, &spec); err != nil {
		return nil, fmt.Errorf("failed to parse OpenAPI spec: %w", err)
	}

	registry := make(SchemaRegistry)
	for _, m := range resourceMappings {
		if len(m.explicitFields) > 0 {
			registry[m.terraformType] = &ResourceSchema{TerraformType: m.terraformType, APIPath: m.apiPath, Fields: m.explicitFields}
			continue
		}
		schema, ok := spec.Components.Schemas[m.schemaName]
		if !ok {
			// Try with "Create" prefix for request schemas.
			schema, ok = spec.Components.Schemas["Create"+m.schemaName]
			if !ok {
				continue
			}
		}

		requiredSet := make(map[string]bool)
		for _, r := range schema.Required {
			requiredSet[r] = true
		}

		var fields []Field
		for propName, prop := range schema.Properties {
			if isComputedOnly(propName) {
				continue
			}
			// Skip fields explicitly excluded for this resource type.
			if m.excludeFields != nil && m.excludeFields[propName] {
				continue
			}

			ft := resolveType(prop)
			itemType := FieldTypeUnknown
			if ft == FieldTypeArray && prop.Items != nil {
				itemType = FieldType(prop.Items.Type)
			}

			fields = append(fields, Field{
				Name:     propName,
				Type:     ft,
				Required: requiredSet[propName],
				Nullable: isNullable(prop),
				ItemType: itemType,
			})
		}

		// Add any extra fields defined in the mapping that aren't in the schema.
		for _, ef := range m.extraFields {
			fields = append(fields, ef)
		}

		registry[m.terraformType] = &ResourceSchema{
			TerraformType: m.terraformType,
			APIPath:       m.apiPath,
			Fields:        fields,
		}
	}

	return registry, nil
}

// resolveType figures out the effective type from an OpenAPI property.
func resolveType(p openAPIProperty) FieldType {
	if p.Type != "" {
		return FieldType(p.Type)
	}
	// Check anyOf/oneOf for nullable patterns like {"anyOf": [{"type": "string"}, {"type": "null"}]}.
	for _, a := range append(p.AnyOf, p.OneOf...) {
		if a.Type != "" && a.Type != "null" {
			return FieldType(a.Type)
		}
	}
	return FieldTypeUnknown
}

func isNullable(p openAPIProperty) bool {
	for _, a := range append(p.AnyOf, p.OneOf...) {
		if a.Type == "null" {
			return true
		}
	}
	return false
}

// computedOnlyFields are fields the API returns but Terraform manages internally.
// These should never appear in generated HCL.
var computedOnlyFields = map[string]bool{
	"id":             true,
	"created":        true,
	"deleted_at":     true,
	"user_id":        true,
	"org_id":         true,
	"_object_org_id": true,
	"updated_at":     true,
	"preview_name":   true,
	"preview_secret": true,
	"key":            true,
	"used":           true,
	"avatar_url":     true,
	"given_name":     true,
	"family_name":    true,
}

func isComputedOnly(field string) bool {
	return computedOnlyFields[field]
}

// resourceMapping connects a Terraform type to an OpenAPI schema and API path.
type resourceMapping struct {
	terraformType  string
	schemaName     string // Key in components.schemas
	apiPath        string
	excludeFields  map[string]bool
	extraFields    []Field
	explicitFields []Field // For provider-only resources or schemas absent from the bundled snapshot.
}

// ref extracts a schema name from a $ref like "#/components/schemas/Foo".
func ref(s string) string {
	parts := strings.Split(s, "/")
	return parts[len(parts)-1]
}

// resourceMappings defines the Terraform type → OpenAPI schema mapping.
var resourceMappings = []resourceMapping{
	{
		terraformType: "braintrust_org_automation",
		apiPath:       "/v1/org_automation",
		explicitFields: []Field{
			{Name: "org_id", Type: FieldTypeString, Required: true},
			{Name: "name", Type: FieldTypeString, Required: true},
			{Name: "description", Type: FieldTypeString},
			{Name: "retention_config", Type: FieldTypeObject, Required: true},
		},
	},
	{
		terraformType: "braintrust_mcp_server",
		apiPath:       "/v1/mcp_server",
		explicitFields: []Field{
			{Name: "project_id", Type: FieldTypeString, Required: true},
			{Name: "name", Type: FieldTypeString, Required: true},
			{Name: "url", Type: FieldTypeString, Required: true},
			{Name: "description", Type: FieldTypeString},
		},
	},
	{
		terraformType: "braintrust_view",
		apiPath:       "/v1/view",
		explicitFields: []Field{
			{Name: "object_type", Type: FieldTypeString, Required: true},
			{Name: "object_id", Type: FieldTypeString, Required: true},
			{Name: "view_type", Type: FieldTypeString, Required: true},
			{Name: "name", Type: FieldTypeString, Required: true},
			{Name: "description", Type: FieldTypeString},
			{Name: "starred", Type: FieldTypeBool},
			{Name: "view_data_json", Type: FieldTypeString},
			{Name: "options_json", Type: FieldTypeString},
		},
	},
	{
		terraformType: "braintrust_project",
		schemaName:    "Project",
		apiPath:       "/v1/project",
		excludeFields: map[string]bool{"settings": true},
	},
	{
		terraformType: "braintrust_dataset",
		schemaName:    "Dataset",
		apiPath:       "/v1/dataset",
		excludeFields: map[string]bool{"metadata": true, "url_slug": true},
		extraFields:   []Field{{Name: "metadata_json", Type: FieldTypeString}},
	},
	{
		terraformType: "braintrust_function",
		schemaName:    "Function",
		apiPath:       "/v1/function",
		excludeFields: map[string]bool{
			"log_id":          true,
			"metadata":        true,
			"origin":          true,
			"prompt_data":     true,
			"function_data":   true,
			"function_schema": true,
		},
		extraFields: []Field{
			{Name: "function_data_json", Type: FieldTypeString, Required: true},
			{Name: "prompt_data_json", Type: FieldTypeString},
			{Name: "function_schema_json", Type: FieldTypeString},
		},
	},
	{
		terraformType: "braintrust_role",
		schemaName:    "Role",
		apiPath:       "/v1/role",
	},
	{
		terraformType: "braintrust_group",
		schemaName:    "Group",
		apiPath:       "/v1/group",
	},
	{
		terraformType: "braintrust_project_group",
		apiPath:       "/v1/project_group",
		explicitFields: []Field{
			{Name: "org_id", Type: FieldTypeString, Required: true},
			{Name: "name", Type: FieldTypeString, Required: true},
			{Name: "description", Type: FieldTypeString, Nullable: true},
			{Name: "member_projects", Type: FieldTypeArray, ItemType: FieldTypeString},
		},
	},
	{
		terraformType: "braintrust_project_group_membership",
		apiPath:       "/v1/project_group",
		explicitFields: []Field{
			{Name: "project_group_id", Type: FieldTypeString, Required: true},
			{Name: "project_id", Type: FieldTypeString, Required: true},
		},
	},
	{
		terraformType: "braintrust_acl",
		schemaName:    "Acl",
		apiPath:       "/v1/acl",
	},
	{
		terraformType: "braintrust_env_var",
		schemaName:    "EnvVar",
		apiPath:       "/v1/env_var",
		excludeFields: map[string]bool{"secret_category": true},
	},
	{
		terraformType: "braintrust_project_tag",
		schemaName:    "ProjectTag",
		apiPath:       "/v1/project_tag",
	},
	{
		terraformType: "braintrust_project_score",
		schemaName:    "ProjectScore",
		apiPath:       "/v1/project_score",
		excludeFields: map[string]bool{"categories": true, "config": true},
		extraFields: []Field{
			{Name: "categories_json", Type: FieldTypeString},
			{Name: "config_json", Type: FieldTypeString},
		},
	},
	{
		terraformType: "braintrust_service_token",
		schemaName:    "ServiceToken",
		apiPath:       "/v1/service_token",
		excludeFields: map[string]bool{
			"service_account_email": true,
			"service_account_name":  true,
		},
	},
	{
		terraformType: "braintrust_ai_secret",
		schemaName:    "AISecret",
		apiPath:       "/v1/ai_secret",
	},
	{
		terraformType: "braintrust_project_automation",
		schemaName:    "ProjectAutomation",
		apiPath:       "/v1/project_automation",
		excludeFields: map[string]bool{"config": true},
		extraFields:   []Field{{Name: "config_json", Type: FieldTypeString}},
	},
}

// _ ensures ref is used (it may be useful for future $ref resolution).
var _ = ref
