package provider

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestJSONStateNumberPrecision(t *testing.T) {
	for _, tc := range []struct {
		name, configured, remote string
		equal                    bool
	}{
		{"normalized number", `{"n":[1.0,1e3]}`, `{"n":[1,1000]}`, true},
		// The API stores numbers as float64; its rounding is not drift.
		{"large integer rounded by API", `{"n":9007199254740993}`, `{"n":9007199254740992}`, true},
		{"long decimal rounded by API", `{"n":0.1234567890123456789}`, `{"n":0.12345678901234568}`, true},
		{"number drift", `{"n":1}`, `{"n":2}`, false},
		{"nested drift", `{"n":[{"keep":true,"remove":null}]}`, `{"n":[{"keep":true}]}`, false},
		{"out of range spelling", `{"n":1e999999999}`, `{"n":1E999999999}`, false},
		{"trailing data", `{"n":1} junk`, `{"n":1}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			existing := types.StringValue(tc.configured)
			got := jsonState(json.RawMessage(tc.remote), existing)
			if got.Equal(existing) != tc.equal {
				t.Fatalf("state = %s, existing = %s, want equal %v", got, existing, tc.equal)
			}
			if scoreJSONEqual(existing, types.StringValue(tc.remote)) != tc.equal {
				t.Fatal("update equality disagrees with state mapping")
			}
			if !tc.equal && got.ValueString() != tc.remote {
				t.Fatalf("remote value not recorded: %s", got)
			}
		})
	}
}

func TestJSONRawToTFStringCanonical(t *testing.T) {
	const remote = `{"b":{"z":1,"a":9007199254740993},"a":0.1234567890123456789}`
	const want = `{"a":0.1234567890123456789,"b":{"a":9007199254740993,"z":1}}`
	if got := jsonRawToTFString(json.RawMessage(remote)); got.ValueString() != want {
		t.Fatalf("got %s, want %s", got, want)
	}
}

func TestJSONValuesEqualHugeExponentIsFast(t *testing.T) {
	start := time.Now()
	a, _ := decodeJSONValue([]byte("1e999999999"))
	b, _ := decodeJSONValue([]byte("10e999999998"))
	if jsonValuesEqual(a, b) {
		t.Fatal("out-of-range numbers with different spellings compared equal")
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("comparison took %s", elapsed)
	}
}
