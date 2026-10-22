package hcl

import "github.com/braintrustdata/terraform-provider-braintrust/internal/terraforming/api"

// ResourceNameFromID generates the canonical Terraform resource name for an ID.
// Uses the first 8 chars of the UUID, sanitized.
func ResourceNameFromID(id string) string {
	return ResourceName(id)
}

// ResourceNameFromIDAndLabel generates a human-friendly resource name.
// Uses a sanitized label with the short ID as suffix to ensure uniqueness.
func ResourceNameFromIDAndLabel(id, label string) string {
	if label == "" {
		return ResourceName(id)
	}
	sanitized := api.SanitizeName(label)
	if sanitized == "" {
		return ResourceName(id)
	}
	short := id
	if len(short) > 8 {
		short = short[:8]
	}
	return sanitized + "_" + api.SanitizeName(short)
}
