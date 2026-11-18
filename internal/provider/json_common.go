package provider

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Preserve the configured JSON spelling when the API returns an equivalent value.
func jsonState(remote json.RawMessage, existing types.String) types.String {
	if !existing.IsNull() && !existing.IsUnknown() && len(remote) > 0 {
		planned, plannedErr := decodeJSONValue([]byte(existing.ValueString()))
		actual, actualErr := decodeJSONValue(remote)
		if plannedErr == nil && actualErr == nil && jsonValuesEqual(planned, actual) {
			return existing
		}
	}
	return jsonRawToTFString(remote)
}

func jsonRawToTFString(raw json.RawMessage) types.String {
	if len(raw) == 0 {
		return types.StringNull()
	}
	if value, err := decodeJSONValue(raw); err == nil {
		if canonical, err := json.Marshal(value); err == nil {
			return types.StringValue(string(canonical))
		}
	}
	return types.StringValue(string(raw))
}

// decodeJSONValue decodes exactly one JSON value, keeping numbers as
// json.Number so re-encoding does not round them. Trailing data is an error,
// matching json.Unmarshal.
func decodeJSONValue(data []byte) (any, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return nil, errors.New("unexpected data after JSON value")
	}
	return value, nil
}

// jsonValuesEqual compares decoded JSON values. Numbers are equal when they
// are spelled the same or parse to the same float64: the API stores JSON
// numbers as float64, so precision beyond that is not observable drift.
func jsonValuesEqual(a, b any) bool {
	switch a := a.(type) {
	case json.Number:
		other, ok := b.(json.Number)
		if !ok {
			return false
		}
		if a == other {
			return true
		}
		left, leftErr := strconv.ParseFloat(string(a), 64)
		right, rightErr := strconv.ParseFloat(string(other), 64)
		return leftErr == nil && rightErr == nil && left == right
	case map[string]any:
		other, ok := b.(map[string]any)
		if !ok || len(a) != len(other) {
			return false
		}
		for key, value := range a {
			actual, exists := other[key]
			if !exists || !jsonValuesEqual(value, actual) {
				return false
			}
		}
		return true
	case []any:
		other, ok := b.([]any)
		if !ok || len(a) != len(other) {
			return false
		}
		for i, value := range a {
			if !jsonValuesEqual(value, other[i]) {
				return false
			}
		}
		return true
	default:
		return reflect.DeepEqual(a, b)
	}
}
