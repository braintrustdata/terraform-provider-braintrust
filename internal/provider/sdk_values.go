package provider

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/oapi-codegen/nullable"
)

// apiTimeLayout matches the timestamps the Braintrust API returns, so values
// decoded into time.Time render the same as the raw API strings did.
const apiTimeLayout = "2006-01-02T15:04:05.000Z07:00"

// nullableStringToTF converts an SDK nullable string to a Terraform value.
// Unspecified and null both map to null.
func nullableStringToTF(v nullable.Nullable[string]) types.String {
	if !v.IsSpecified() || v.IsNull() {
		return types.StringNull()
	}
	return types.StringValue(v.MustGet())
}

// nullableUUIDToTF converts an SDK nullable UUID to a Terraform value.
func nullableUUIDToTF(v nullable.Nullable[uuid.UUID]) types.String {
	if !v.IsSpecified() || v.IsNull() {
		return types.StringNull()
	}
	return types.StringValue(v.MustGet().String())
}

// nullableTimeToTF converts an SDK nullable timestamp to a Terraform value.
// A prior value naming the same instant is kept as-is, so state written from
// the raw API string never changes just because of formatting.
func nullableTimeToTF(v nullable.Nullable[time.Time], prior types.String) types.String {
	if !v.IsSpecified() || v.IsNull() {
		return types.StringNull()
	}
	t := v.MustGet()
	if !prior.IsNull() && !prior.IsUnknown() {
		if p, err := time.Parse(time.RFC3339Nano, prior.ValueString()); err == nil && p.Equal(t) {
			return prior
		}
	}
	return types.StringValue(t.UTC().Format(apiTimeLayout))
}

// tfStringToNullable converts a Terraform value to an SDK nullable string.
// Null and unknown values are left unspecified so they are omitted from requests.
func tfStringToNullable(s types.String) nullable.Nullable[string] {
	if s.IsNull() || s.IsUnknown() {
		return nullable.Nullable[string]{}
	}
	return nullable.NewNullableWithValue(s.ValueString())
}

// nullableIsSet reports whether an SDK nullable holds a non-null value.
func nullableIsSet[T any](v nullable.Nullable[T]) bool {
	return v.IsSpecified() && !v.IsNull()
}

// patchString builds a PATCH field for an optional string attribute: the
// planned value when set, an empty string to clear a removed value, and
// unspecified (omitted) otherwise.
func patchString(plan, state types.String) nullable.Nullable[string] {
	if !plan.IsNull() && !plan.IsUnknown() {
		return nullable.NewNullableWithValue(plan.ValueString())
	}
	if plan.IsNull() && !state.IsNull() {
		return nullable.NewNullableWithValue("")
	}
	return nullable.Nullable[string]{}
}

// parseUUID parses a Terraform ID attribute for an SDK request.
func parseUUID(value types.String, attribute string) (uuid.UUID, error) {
	id, err := uuid.Parse(value.ValueString())
	if err != nil {
		return uuid.UUID{}, fmt.Errorf("%s must be a UUID: %w", attribute, err)
	}
	return id, nil
}

// uuidStrings formats SDK UUIDs as Terraform ID strings.
func uuidStrings(ids []uuid.UUID) []string {
	result := make([]string, len(ids))
	for i, id := range ids {
		result[i] = id.String()
	}
	return result
}

// parseUUIDs parses Terraform ID strings for an SDK request.
func parseUUIDs(values []string, attribute string) ([]uuid.UUID, error) {
	result := make([]uuid.UUID, len(values))
	for i, value := range values {
		id, err := uuid.Parse(value)
		if err != nil {
			return nil, fmt.Errorf("%s must contain UUIDs: %q: %w", attribute, value, err)
		}
		result[i] = id
	}
	return result, nil
}

// nullableSlice wraps a slice for an SDK request, leaving it unspecified
// (omitted) when empty.
func nullableSlice[T any](values []T) nullable.Nullable[[]T] {
	if len(values) == 0 {
		return nullable.Nullable[[]T]{}
	}
	return nullable.NewNullableWithValue(values)
}

// nullableSliceValue returns the slice in an SDK nullable, or nil.
func nullableSliceValue[T any](v nullable.Nullable[[]T]) []T {
	if !nullableIsSet(v) {
		return nil
	}
	return v.MustGet()
}

// diffUUIDs computes the IDs to add and remove to go from current to desired,
// parsed for an SDK request.
func diffUUIDs(current, desired []string, attribute string) (add, remove []uuid.UUID, err error) {
	addIDs, removeIDs := diffStringSlices(current, desired)
	if add, err = parseUUIDs(addIDs, attribute); err != nil {
		return nil, nil, err
	}
	if remove, err = parseUUIDs(removeIDs, attribute); err != nil {
		return nil, nil, err
	}
	return add, remove, nil
}

// tfStringToNullableUUID parses an optional Terraform ID attribute for an SDK
// request. Null and unknown values are left unspecified.
func tfStringToNullableUUID(s types.String, attribute string) (nullable.Nullable[uuid.UUID], error) {
	if s.IsNull() || s.IsUnknown() {
		return nullable.Nullable[uuid.UUID]{}, nil
	}
	id, err := parseUUID(s, attribute)
	if err != nil {
		return nullable.Nullable[uuid.UUID]{}, err
	}
	return nullable.NewNullableWithValue(id), nil
}

// tfStringToEnum converts an optional Terraform string to an SDK enum pointer.
func tfStringToEnum[T ~string](s types.String) *T {
	if s.IsNull() || s.IsUnknown() {
		return nil
	}
	v := T(s.ValueString())
	return &v
}

// enumToTF converts an optional SDK enum to a Terraform value.
func enumToTF[T ~string](v *T) types.String {
	if v == nil {
		return types.StringNull()
	}
	return types.StringValue(string(*v))
}
