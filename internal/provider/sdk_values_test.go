package provider

import (
	"testing"
	"time"

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
