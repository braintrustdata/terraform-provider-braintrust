package provider

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/braintrustdata/braintrust-sdk-go/openapi"
	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/oapi-codegen/nullable"
)

func TestNullableTimeToTFKeepsEquivalentPriorValue(t *testing.T) {
	ts := nullable.NewNullableWithValue(time.Date(2026, 10, 21, 20, 37, 22, 135_000_000, time.UTC))
	if got := nullableTimeToTF(ts, types.StringUnknown()).ValueString(); got != "2026-10-21T20:37:22.135Z" {
		t.Fatalf("want API format, got %q", got)
	}
	prior := types.StringValue("2026-10-21T20:37:22.135000+00:00")
	if got := nullableTimeToTF(ts, prior); !got.Equal(prior) {
		t.Fatalf("an equivalent prior value must be kept; got %q", got.ValueString())
	}
	if got := nullableTimeToTF(ts, types.StringValue("2026-01-01T00:00:00.000Z")).ValueString(); got != "2026-10-21T20:37:22.135Z" {
		t.Fatalf("a different prior value must be replaced; got %q", got)
	}
}

func TestNullableTimeToTFKeepsSubMillisecondPrecision(t *testing.T) {
	ts := nullable.NewNullableWithValue(time.Date(2024, 5, 1, 12, 34, 56, 789_123_000, time.UTC))
	if got := nullableTimeToTF(ts, types.StringNull()).ValueString(); got != "2024-05-01T12:34:56.789123Z" {
		t.Fatalf("sub-millisecond precision must be kept; got %q", got)
	}
}

func TestOptionalUUIDToTFMapsZeroToNull(t *testing.T) {
	if got := optionalUUIDToTF(uuid.Nil); !got.IsNull() {
		t.Fatalf("a missing or null UUID must be null; got %q", got.ValueString())
	}
	id := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	if got := optionalUUIDToTF(id).ValueString(); got != id.String() {
		t.Fatalf("got %q", got)
	}
}

func TestMapProjectTagNullUserID(t *testing.T) {
	var tag openapi.ProjectTag
	if err := json.Unmarshal([]byte(`{"id":"00000000-0000-0000-0000-000000000001","project_id":"00000000-0000-0000-0000-000000000002","user_id":null,"name":"tag"}`), &tag); err != nil {
		t.Fatal(err)
	}
	var model projectTagModel
	mapProjectTagToModel(&tag, &model)
	if !model.UserID.IsNull() {
		t.Fatalf("a null user_id must stay null; got %q", model.UserID.ValueString())
	}
}
