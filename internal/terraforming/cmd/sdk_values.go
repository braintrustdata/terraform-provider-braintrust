package cmd

import (
	"github.com/google/uuid"
	"github.com/oapi-codegen/nullable"
)

// optional returns an SDK nullable's value and whether it holds one.
func optional[T any](v nullable.Nullable[T]) (T, bool) {
	if !v.IsSpecified() || v.IsNull() {
		var zero T
		return zero, false
	}
	return v.MustGet(), true
}

// optionalUUID returns an SDK nullable UUID as a string ID.
func optionalUUID(v nullable.Nullable[uuid.UUID]) (string, bool) {
	id, ok := optional(v)
	if !ok {
		return "", false
	}
	return id.String(), true
}

// uuidStrings formats SDK UUIDs as string IDs.
func uuidStrings(ids []uuid.UUID) []string {
	result := make([]string, len(ids))
	for i, id := range ids {
		result[i] = id.String()
	}
	return result
}

// optionalEnum returns an SDK enum pointer's value as a string.
func optionalEnum[T ~string](v *T) (string, bool) {
	if v == nil {
		return "", false
	}
	return string(*v), true
}
