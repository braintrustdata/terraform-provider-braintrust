package schema

// WritableFields returns the fields for a resource type that should be written
// to Terraform configuration. This is the primary interface used by the HCL writer.
func (r *ResourceSchema) WritableFields() []Field {
	return r.Fields
}

// FieldByName returns a field by name, or nil if not found.
func (r *ResourceSchema) FieldByName(name string) *Field {
	for i := range r.Fields {
		if r.Fields[i].Name == name {
			return &r.Fields[i]
		}
	}
	return nil
}

// linkableFields maps Terraform resource types to the field names that reference them.
// When linking is enabled for a type, these fields become Terraform references.
var linkableFields = map[string]string{
	"braintrust_project": "project_id",
	"braintrust_role":    "role_id",
	"braintrust_group":   "group_id",
}

// LinkField returns the field name that links to this resource type,
// or empty string if this type is not linkable.
func LinkField(terraformType string) string {
	return linkableFields[terraformType]
}
