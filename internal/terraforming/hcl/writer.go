// Package hcl generates Terraform HCL configuration from Braintrust API data.
package hcl

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/braintrustdata/terraform-provider-braintrust/internal/terraforming/api"
	"github.com/braintrustdata/terraform-provider-braintrust/internal/terraforming/schema"
	hcl2 "github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/zclconf/go-cty/cty"
)

// ResourceBlock represents a single Terraform resource to be written.
type ResourceBlock struct {
	Type         string            // e.g. "braintrust_project"
	Name         string            // e.g. "terraform_managed_resource_abc123"
	Comment      string            // Optional comment (e.g. the human-readable name)
	ID           string            // The resource ID (for import blocks)
	Fields       map[string]any    // Field name → value from the API
	LinkedFields map[string]string // Field name → Terraform reference expression
}

// Writer produces HCL output.
type Writer struct {
	registry schema.SchemaRegistry
}

// DataBlock represents a Terraform data source to be written.
type DataBlock struct {
	Type    string
	Name    string
	Comment string
	Fields  map[string]any
}

// NewWriter creates a new HCL writer.
func NewWriter(registry schema.SchemaRegistry) *Writer {
	return &Writer{registry: registry}
}

// WriteDataSources writes Terraform data blocks.
func (w *Writer) WriteDataSources(blocks []DataBlock) []byte {
	if len(blocks) == 0 {
		return nil
	}

	f := hclwrite.NewEmptyFile()
	body := f.Body()

	for i, block := range blocks {
		if i > 0 {
			body.AppendNewline()
		}
		if block.Comment != "" {
			body.AppendUnstructuredTokens(comment(block.Comment))
		}

		dataBlock := body.AppendNewBlock("data", []string{block.Type, block.Name})
		dataBody := dataBlock.Body()

		keys := make([]string, 0, len(block.Fields))
		for k := range block.Fields {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			writeValue(dataBody, k, schema.FieldTypeUnknown, block.Fields[k])
		}
	}

	return hclwrite.Format(f.Bytes())
}

// WriteResources writes a set of resource blocks to HCL.
func (w *Writer) WriteResources(blocks []ResourceBlock) []byte {
	f := hclwrite.NewEmptyFile()
	body := f.Body()

	for i, block := range blocks {
		if i > 0 {
			body.AppendNewline()
		}

		if block.Comment != "" {
			body.AppendUnstructuredTokens(comment(block.Comment))
		}

		rs := w.registry[block.Type]
		resourceBlock := body.AppendNewBlock("resource", []string{block.Type, block.Name})
		resourceBody := resourceBlock.Body()

		if rs != nil {
			w.writeFieldsFromSchema(resourceBody, rs, block)
		} else {
			w.writeFieldsRaw(resourceBody, block)
		}
	}

	return hclwrite.Format(f.Bytes())
}

// WriteImports writes Terraform 1.5+ import blocks.
func (w *Writer) WriteImports(blocks []ResourceBlock) []byte {
	f := hclwrite.NewEmptyFile()
	body := f.Body()

	for i, block := range blocks {
		if i > 0 {
			body.AppendNewline()
		}

		importBlock := body.AppendNewBlock("import", nil)
		importBody := importBlock.Body()

		toExpr := fmt.Sprintf("%s.%s", block.Type, block.Name)
		importBody.SetAttributeRaw("to", hclRawRef(toExpr))
		importBody.SetAttributeValue("id", cty.StringVal(block.ID))
	}

	return hclwrite.Format(f.Bytes())
}

// WriteVariables writes variable blocks for secrets.
func (w *Writer) WriteVariables(vars []VariableBlock) []byte {
	if len(vars) == 0 {
		return nil
	}
	f := hclwrite.NewEmptyFile()
	body := f.Body()

	for i, v := range vars {
		if i > 0 {
			body.AppendNewline()
		}

		varBlock := body.AppendNewBlock("variable", []string{v.Name})
		varBody := varBlock.Body()
		if v.Description != "" {
			varBody.SetAttributeValue("description", cty.StringVal(v.Description))
		}
		varBody.SetAttributeRaw("type", hclRawExpr("string"))
		varBody.SetAttributeValue("sensitive", cty.BoolVal(true))
	}

	return hclwrite.Format(f.Bytes())
}

// VariableBlock represents a Terraform variable for secrets.
type VariableBlock struct {
	Name        string
	Description string
}

// RawExpression is a raw HCL expression string.
type RawExpression string

// writeFieldsFromSchema writes fields guided by the provider schema.
func (w *Writer) writeFieldsFromSchema(body *hclwrite.Body, rs *schema.ResourceSchema, block ResourceBlock) {
	// Sort fields: required first, then alphabetical.
	fields := rs.WritableFields()
	sort.Slice(fields, func(i, j int) bool {
		if fields[i].Required != fields[j].Required {
			return fields[i].Required
		}
		return fields[i].Name < fields[j].Name
	})

	written := make(map[string]bool)

	for _, field := range fields {
		// Check if this field has a linked reference.
		if ref, ok := block.LinkedFields[field.Name]; ok {
			body.SetAttributeRaw(field.Name, hclRawRef(ref))
			written[field.Name] = true
			continue
		}

		val, ok := block.Fields[field.Name]
		if !ok || val == nil {
			continue
		}

		writeValue(body, field.Name, field.Type, val)
		written[field.Name] = true
	}
	// Builders provide provider attributes explicitly. Some write-only and
	// provider-specific attributes do not appear in the API response schema.
	keys := make([]string, 0, len(block.Fields))
	for k := range block.Fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if !written[k] && block.Fields[k] != nil {
			if _, linked := block.LinkedFields[k]; !linked {
				writeValue(body, k, schema.FieldTypeUnknown, block.Fields[k])
			}
		}
	}

	// Write any linked fields that weren't in the schema (e.g., write-only fields
	// like "secret" that don't appear in API responses but need variable refs).
	linkedKeys := make([]string, 0, len(block.LinkedFields))
	for k := range block.LinkedFields {
		linkedKeys = append(linkedKeys, k)
	}
	sort.Strings(linkedKeys)
	for _, k := range linkedKeys {
		if !written[k] {
			body.SetAttributeRaw(k, hclRawRef(block.LinkedFields[k]))
		}
	}
}

// writeFieldsRaw writes fields without schema guidance (fallback).
func (w *Writer) writeFieldsRaw(body *hclwrite.Body, block ResourceBlock) {
	keys := make([]string, 0, len(block.Fields))
	for k := range block.Fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		if ref, ok := block.LinkedFields[k]; ok {
			body.SetAttributeRaw(k, hclRawRef(ref))
			continue
		}
		val := block.Fields[k]
		if val == nil {
			continue
		}
		writeValue(body, k, schema.FieldTypeUnknown, val)
	}
}

// writeValue writes a single attribute value to an HCL body.
func writeValue(body *hclwrite.Body, name string, ft schema.FieldType, val any) {
	switch v := val.(type) {
	case string:
		if strings.HasSuffix(name, "_json") {
			var parsed any
			if json.Unmarshal([]byte(v), &parsed) == nil {
				body.SetAttributeRaw(name, hclwrite.TokensForFunctionCall("jsonencode", hclwrite.TokensForValue(toCtyValue(parsed))))
				return
			}
		}
		body.SetAttributeValue(name, cty.StringVal(v))
	case RawExpression:
		body.SetAttributeRaw(name, hclRawExpr(string(v)))
	case float64:
		if ft == schema.FieldTypeInteger || v == float64(int64(v)) {
			body.SetAttributeValue(name, cty.NumberIntVal(int64(v)))
		} else {
			body.SetAttributeValue(name, cty.NumberFloatVal(v))
		}
	case bool:
		body.SetAttributeValue(name, cty.BoolVal(v))
	case []any:
		writeArrayValue(body, name, v)
	case []string:
		if len(v) == 0 {
			return
		}
		vals := make([]cty.Value, len(v))
		for i, s := range v {
			vals[i] = cty.StringVal(s)
		}
		body.SetAttributeValue(name, cty.ListVal(vals))
	case json.RawMessage:
		// For raw JSON (like automation config), write as an HCL expression.
		var parsed any
		if err := json.Unmarshal(v, &parsed); err == nil {
			writeValue(body, name, ft, parsed)
		}
	case map[string]any:
		writeMapValue(body, name, v)
	default:
		// Try converting to string as a last resort.
		body.SetAttributeValue(name, cty.StringVal(fmt.Sprintf("%v", val)))
	}
}

// writeArrayValue writes an array attribute.
func writeArrayValue(body *hclwrite.Body, name string, arr []any) {
	if len(arr) == 0 {
		return
	}

	hasRaw := false
	allRawCapable := true
	for _, item := range arr {
		switch item.(type) {
		case string, RawExpression:
		default:
			allRawCapable = false
		}
		if _, ok := item.(RawExpression); ok {
			hasRaw = true
		}
	}
	if hasRaw && allRawCapable {
		parts := make([]string, 0, len(arr))
		for _, item := range arr {
			switch v := item.(type) {
			case string:
				parts = append(parts, fmt.Sprintf("%q", v))
			case RawExpression:
				parts = append(parts, string(v))
			}
		}
		body.SetAttributeRaw(name, hclRawExpr("["+strings.Join(parts, ", ")+"]"))
		return
	}

	// Check if all elements are strings.
	allStrings := true
	vals := make([]cty.Value, len(arr))
	for i, item := range arr {
		s, ok := item.(string)
		if !ok {
			allStrings = false
			break
		}
		vals[i] = cty.StringVal(s)
	}
	if allStrings {
		body.SetAttributeValue(name, cty.ListVal(vals))
		return
	}

	// For arrays of objects (like member_permissions), write as dynamic blocks.
	// For now, use JSON encoding as a comment placeholder.
	// This handles the common case of member_permissions.
	if name == "member_permissions" {
		writeMemberPermissions(body, arr)
		return
	}

	for i, item := range arr {
		vals[i] = toCtyValue(item)
	}
	body.SetAttributeValue(name, cty.TupleVal(vals))
}

// writeMemberPermissions handles the special member_permissions array.
func writeMemberPermissions(body *hclwrite.Body, perms []any) {
	// member_permissions in Terraform is a list of objects.
	vals := make([]cty.Value, 0, len(perms))
	for _, p := range perms {
		pm, ok := p.(map[string]any)
		if !ok {
			continue
		}
		perm, _ := pm["permission"].(string)
		if perm == "" {
			continue
		}
		obj := map[string]cty.Value{
			"permission": cty.StringVal(perm),
		}
		if rot, ok := pm["restrict_object_type"].(string); ok && rot != "" {
			obj["restrict_object_type"] = cty.StringVal(rot)
		}
		vals = append(vals, cty.ObjectVal(obj))
	}
	if len(vals) > 0 {
		body.SetAttributeValue("member_permissions", cty.TupleVal(vals))
	}
}

// writeMapValue writes a map attribute.
func writeMapValue(body *hclwrite.Body, name string, m map[string]any) {
	vals := make(map[string]cty.Value)
	for k, v := range m {
		vals[k] = toCtyValue(v)
	}
	body.SetAttributeValue(name, cty.ObjectVal(vals))
}

func toCtyValue(v any) cty.Value {
	switch x := v.(type) {
	case nil:
		return cty.NullVal(cty.DynamicPseudoType)
	case string:
		return cty.StringVal(x)
	case float64:
		return cty.NumberFloatVal(x)
	case int:
		return cty.NumberIntVal(int64(x))
	case bool:
		return cty.BoolVal(x)
	case []any:
		vals := make([]cty.Value, len(x))
		for i, item := range x {
			vals[i] = toCtyValue(item)
		}
		return cty.TupleVal(vals)
	case map[string]any:
		vals := make(map[string]cty.Value, len(x))
		for k, item := range x {
			vals[k] = toCtyValue(item)
		}
		return cty.ObjectVal(vals)
	default:
		return cty.StringVal(fmt.Sprint(v))
	}
}

// comment produces HCL comment tokens.
func comment(text string) hclwrite.Tokens {
	var tokens hclwrite.Tokens
	for _, line := range strings.Split(strings.ReplaceAll(text, "\r", ""), "\n") {
		tokens = append(tokens, &hclwrite.Token{
			Type:  hclsyntax.TokenComment,
			Bytes: []byte("# " + line + "\n"),
		})
	}
	return tokens
}

// hclRawRef produces tokens for a raw Terraform reference (unquoted).
func hclRawRef(ref string) hclwrite.Tokens {
	return hclRawExpr(ref)
}

func hclRawExpr(expr string) hclwrite.Tokens {
	tokens, diags := hclsyntax.LexExpression([]byte(expr), "", hcl2.Pos{Line: 1, Column: 1})
	if diags.HasErrors() {
		return hclwrite.TokensForTraversal(parseTraversal(expr))
	}
	out := make(hclwrite.Tokens, 0, len(tokens))
	for _, tok := range tokens {
		out = append(out, &hclwrite.Token{
			Type:  tok.Type,
			Bytes: tok.Bytes,
		})
	}
	return out
}

// parseTraversal converts a dotted reference like "braintrust_project.foo.id"
// into hclwrite tokens. Falls back to raw tokens for non-standard expressions
// like "var.foo".
func parseTraversal(ref string) hcl2.Traversal {
	parts := splitDotRef(ref)
	if len(parts) == 0 {
		return hcl2.Traversal{hcl2.TraverseRoot{Name: ref}}
	}
	t := hcl2.Traversal{hcl2.TraverseRoot{Name: parts[0]}}
	for _, p := range parts[1:] {
		t = append(t, hcl2.TraverseAttr{Name: p})
	}
	return t
}

func splitDotRef(s string) []string {
	var parts []string
	current := ""
	for _, c := range s {
		if c == '.' {
			if current != "" {
				parts = append(parts, current)
				current = ""
			}
		} else {
			current += string(c)
		}
	}
	if current != "" {
		parts = append(parts, current)
	}
	return parts
}

// ResourceName generates the Terraform resource name for a given ID.
func ResourceName(id string) string {
	// Truncate UUID to first 8 chars for readability.
	short := id
	if len(short) > 8 {
		short = short[:8]
	}
	return "terraform_managed_resource_" + api.SanitizeName(short)
}
