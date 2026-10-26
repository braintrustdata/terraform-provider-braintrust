package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

// The SDK models only know the fields in their pinned spec. Free-form JSON
// that Terraform passes through (user-supplied config, or settings the
// provider does not manage) must not round-trip through those models, or
// unknown keys are dropped. These helpers keep such fields as raw JSON.

// RawFields decodes the top-level fields of a JSON object without
// interpreting their values.
func RawFields(body []byte) (map[string]json.RawMessage, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(body, &fields); err != nil {
		return nil, fmt.Errorf("failed to decode response fields: %w", err)
	}
	return fields, nil
}

// Raw pairs a decoded SDK object with the raw JSON of its top-level fields,
// for fields that must be read without going through the SDK model.
type Raw[T any] struct {
	Value  *T
	Fields map[string]json.RawMessage
}

// DecodeRaw combines a generated response's decoded body with its raw fields.
func DecodeRaw[T any](value *T, body []byte) (*Raw[T], error) {
	decoded, err := JSON(value)
	if err != nil {
		return nil, err
	}
	fields, err := RawFields(body)
	if err != nil {
		return nil, err
	}
	return &Raw[T]{Value: decoded, Fields: fields}, nil
}

// JSONBody encodes a typed SDK request and sets the given top-level fields to
// raw JSON values, overriding the typed encoding. Nil values are skipped.
func JSONBody(typed any, raw map[string]json.RawMessage) (io.Reader, error) {
	encoded, err := json.Marshal(typed)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request body: %w", err)
	}
	if len(raw) > 0 {
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(encoded, &fields); err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
		for name, value := range raw {
			if value != nil {
				fields[name] = value
			}
		}
		if encoded, err = json.Marshal(fields); err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
	}
	return bytes.NewReader(encoded), nil
}

// JSONContentType is the content type for generated ...WithBody operations.
const JSONContentType = "application/json"

// RawObjects pairs a decoded list page with the raw JSON of each object.
func RawObjects[T any](objects []T, body []byte) ([]Raw[T], error) {
	var page struct {
		Objects []json.RawMessage `json:"objects"`
	}
	if err := json.Unmarshal(body, &page); err != nil {
		return nil, fmt.Errorf("failed to decode list response: %w", err)
	}
	if len(page.Objects) != len(objects) {
		return nil, fmt.Errorf("list response has %d raw objects for %d decoded objects", len(page.Objects), len(objects))
	}
	result := make([]Raw[T], len(objects))
	for i := range objects {
		fields, err := RawFields(page.Objects[i])
		if err != nil {
			return nil, err
		}
		result[i] = Raw[T]{Value: &objects[i], Fields: fields}
	}
	return result, nil
}
