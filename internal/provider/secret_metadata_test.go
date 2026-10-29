package provider

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// Exercise the real Terraform lifecycle against the backend's two different
// metadata write contracts: AI secrets merge, while env vars replace/clear.
func TestSecretMetadataLifecycle(t *testing.T) {
	for _, kind := range []string{"ai_secret", "env_var"} {
		t.Run(kind, func(t *testing.T) {
			const id = "00000000-0000-4000-8000-000000000001"
			const orgID = "00000000-0000-4000-8000-000000000002"
			object := map[string]any{"id": id, "name": "metadata-test", "metadata": nil}
			if kind == "ai_secret" {
				object["org_id"], object["type"] = orgID, "azure"
			} else {
				object["object_type"], object["object_id"] = "project", orgID
				object["secret_type"], object["secret_category"] = "azure", "ai_provider"
			}
			writes := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch r.Method {
				case http.MethodPut, http.MethodPatch:
					writes++
					var body map[string]json.RawMessage
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Error(err)
						w.WriteHeader(400)
						return
					}
					if raw, ok := body["metadata"]; ok {
						var metadata any
						if err := json.Unmarshal(raw, &metadata); err != nil {
							t.Error(err)
							return
						}
						if kind == "ai_secret" && r.Method == http.MethodPatch {
							if metadata != nil {
								object["metadata"] = mergeSecretMetadata(object["metadata"], metadata)
							}
						} else {
							object["metadata"] = metadata
						}
					}
				case http.MethodGet:
				case http.MethodDelete:
					w.WriteHeader(http.StatusNoContent)
					return
				default:
					t.Errorf("unexpected method %s", r.Method)
					w.WriteHeader(400)
					return
				}
				if err := json.NewEncoder(w).Encode(object); err != nil {
					t.Error(err)
				}
			}))
			defer server.Close()
			config := func(metadata string) string {
				return fmt.Sprintf("provider \"braintrust\" {\n api_key = \"fixture\"\n api_url = %q\n}\n%s", server.URL, secretMetadataConfig(kind, "metadata-test", orgID, metadata))
			}
			rn := "braintrust_" + kind + ".test"
			initial := `{"additionalHeaders":{"x-one":"one","x-two":"two"},"api_base":"https://example.openai.azure.com","api_version":"2024-10-21","deployment":"first"}`
			updated := `{"api_base":"https://example.openai.azure.com","api_version":"2024-10-21","deployment":"second","additionalHeaders":{"x-one":"changed","x-two":"two"}}`
			attr := func(raw string) string { return fmt.Sprintf("metadata = %q", raw) }
			steps := []resource.TestStep{
				{Config: config(""), Check: resource.TestCheckNoResourceAttr(rn, "metadata")},
				{Config: config("metadata = null"), PlanOnly: true},
				{Config: config(`metadata = jsonencode({})`), Check: resource.TestCheckResourceAttr(rn, "metadata", "{}")},
				{Config: config(attr(initial)), Check: resource.TestCheckResourceAttr(rn, "metadata", initial)},
				{ResourceName: rn, ImportState: true, ImportStateVerify: true, ImportStateVerifyIgnore: []string{"secret", "value"}},
				// The server reorders object keys. Refresh must retain the configured JSON.
				{Config: config(attr(initial)), PlanOnly: true},
				{Config: config(attr(updated)), Check: resource.TestCheckResourceAttr(rn, "metadata", updated)},
				{Config: config(attr(updated)), PlanOnly: true},
			}
			if kind == "ai_secret" {
				for _, metadata := range []string{"", "metadata = null", "metadata = jsonencode({})", attr(`{"api_base":"https://example.openai.azure.com","api_version":"2024-10-21","deployment":"second","additionalHeaders":{"x-one":"changed"}}`)} {
					steps = append(steps, resource.TestStep{Config: config(metadata), ExpectError: regexp.MustCompile("Cannot Remove AI Secret Metadata")})
				}
				steps = append(steps, resource.TestStep{Config: config(attr(updated)), PlanOnly: true})
			} else {
				steps = append(steps,
					resource.TestStep{Config: config("metadata = jsonencode({})"), Check: resource.TestCheckResourceAttr(rn, "metadata", "{}")},
					resource.TestStep{Config: config(""), Check: resource.TestCheckNoResourceAttr(rn, "metadata")},
					resource.TestStep{Config: config(""), PlanOnly: true},
					resource.TestStep{ResourceName: rn, ImportState: true, ImportStateVerify: true, ImportStateVerifyIgnore: []string{"value"}},
				)
			}
			resource.UnitTest(t, resource.TestCase{ProtoV6ProviderFactories: testAccProtoV6ProviderFactories, Steps: steps})
			// Also exercise metadata in the initial PUT, including the subsequent
			// read and import, rather than only setting it through PATCH.
			resource.UnitTest(t, resource.TestCase{ProtoV6ProviderFactories: testAccProtoV6ProviderFactories, Steps: []resource.TestStep{
				{Config: config(attr(initial)), Check: resource.TestCheckResourceAttr(rn, "metadata", initial)},
				{ResourceName: rn, ImportState: true, ImportStateVerify: true, ImportStateVerifyIgnore: []string{"secret", "value"}},
				{Config: config(attr(initial)), PlanOnly: true},
			}})
			wantWrites := 5
			if kind == "env_var" {
				wantWrites = 7
			}
			if writes != wantWrites {
				t.Fatalf("got %d writes, want %d; removal errors and re-plans must not write", writes, wantWrites)
			}
		})
	}
}

func mergeSecretMetadata(old, update any) any {
	previous, oldObject := old.(map[string]any)
	next, newObject := update.(map[string]any)
	if !oldObject && newObject && len(next) == 0 {
		return nil
	}
	if !oldObject || !newObject {
		return update
	}
	if len(previous) == 0 && len(next) == 0 {
		return nil
	}
	for key, value := range next {
		previous[key] = mergeSecretMetadata(previous[key], value)
	}
	return previous
}

func secretMetadataConfig(kind, name, projectID, metadata string) string {
	if kind == "ai_secret" {
		return fmt.Sprintf(`resource "braintrust_ai_secret" "test" {
 name = %q
 type = "azure"
 secret = "test-secret"
 %s
}
`, name, metadata)
	}
	return fmt.Sprintf(`resource "braintrust_env_var" "test" {
 name = %q
 object_type = "project"
 object_id = %q
 secret_type = "azure"
 value = "test-secret"
 %s
}
`, name, projectID, metadata)
}

func TestSecretMetadataReadState(t *testing.T) {
	for _, tc := range []struct {
		name, remote, existing, want string
		null                         bool
	}{
		{name: "omitted", null: true},
		{name: "null", remote: "null", null: true},
		{name: "empty", remote: "{}", want: "{}"},
		{name: "format", remote: `{"b":2,"a":1}`, existing: `{ "a": 1, "b": 2 }`, want: `{ "a": 1, "b": 2 }`},
		{name: "drift", remote: `{"deployment":"changed"}`, existing: `{"deployment":"old"}`, want: `{"deployment":"changed"}`},
		{name: "unknown fields", remote: `{"future":{"keys":[true,null,"value"]}}`, want: `{"future":{"keys":[true,null,"value"]}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			existing := types.StringNull()
			if tc.existing != "" {
				existing = types.StringValue(tc.existing)
			}
			got := optionalFunctionJSONState(json.RawMessage(tc.remote), existing)
			if got.IsNull() != tc.null || (!tc.null && got.ValueString() != tc.want) {
				t.Fatalf("got %s, want %q (null=%v)", got, tc.want, tc.null)
			}
		})
	}
}

func TestSecretMetadataInvalidObject(t *testing.T) {
	for _, kind := range []string{"ai_secret", "env_var"} {
		for _, value := range []string{"[]", "null", `"string"`, "1", "{invalid"} {
			t.Run(kind+"/"+value, func(t *testing.T) {
				config := "provider \"braintrust\" {\n api_key = \"fixture\"\n api_url = \"http://localhost:1\"\n}\n" + secretMetadataConfig(kind, "invalid", "00000000-0000-4000-8000-000000000001", fmt.Sprintf("metadata = %q", value))
				resource.UnitTest(t, resource.TestCase{ProtoV6ProviderFactories: testAccProtoV6ProviderFactories, Steps: []resource.TestStep{{Config: config, ExpectError: regexp.MustCompile("metadata must be a JSON object")}}})
			})
		}
	}
}

func TestAISecretMetadataEmptyNormalization(t *testing.T) {
	if got := aiSecretMetadataState(json.RawMessage("null"), types.StringValue("{ }")); got.ValueString() != "{ }" {
		t.Fatalf("empty metadata lost: %s", got)
	}
	if got := aiSecretMetadataState(json.RawMessage("null"), types.StringValue(`{"deployment":"old"}`)); !got.IsNull() {
		t.Fatalf("remote drift hidden: %s", got)
	}
}
