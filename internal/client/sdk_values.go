package client

// Helpers for reading optional SDK values, shared by the provider and the CLI.

import (
	"github.com/google/uuid"
	"github.com/oapi-codegen/nullable"
)

// Optional returns an SDK nullable's value and whether it holds one.
func Optional[T any](v nullable.Nullable[T]) (T, bool) {
	if !v.IsSpecified() || v.IsNull() {
		var zero T
		return zero, false
	}
	return v.MustGet(), true
}

// OptionalUUID returns an SDK nullable UUID as a string ID.
func OptionalUUID(v nullable.Nullable[uuid.UUID]) (string, bool) {
	id, ok := Optional(v)
	if !ok {
		return "", false
	}
	return id.String(), true
}

// UUIDStrings formats SDK UUIDs as string IDs.
func UUIDStrings(ids []uuid.UUID) []string {
	result := make([]string, len(ids))
	for i, id := range ids {
		result[i] = id.String()
	}
	return result
}

// OptionalEnum returns an SDK enum pointer's value as a string.
func OptionalEnum[T ~string](v *T) (string, bool) {
	if v == nil {
		return "", false
	}
	return string(*v), true
}
