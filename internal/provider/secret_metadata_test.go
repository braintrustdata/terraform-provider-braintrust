package provider

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// secretMetadataServer fakes the AI secret and env var endpoints. AI-secret
// PATCH deep-merges metadata unless the replace flag is sent; env-var PATCH
// replaces it. With mergeOnly, PATCH always merges, like an API without
// shallow_replace_json_fields.
func secretMetadataServer(t *testing.T, kind string, mergeOnly bool, defaultMetadata json.RawMessage) (*httptest.Server, map[string]json.RawMessage) {
	t.Helper()
	object := map[string]json.RawMessage{"id": json.RawMessage(fmt.Sprintf("%q", secretMetadataID)), "name": json.RawMessage(`"metadata-test"`)}
	if defaultMetadata != nil {
		object["metadata"] = defaultMetadata
	}
	if kind == "ai_secret" {
		object["org_id"] = json.RawMessage(fmt.Sprintf("%q", secretMetadataOrgID))
		object["type"] = json.RawMessage(`"openai"`)
	} else {
		object["object_type"] = json.RawMessage(`"project"`)
		object["object_id"] = json.RawMessage(fmt.Sprintf("%q", secretMetadataOrgID))
		object["secret_type"] = json.RawMessage(`"openai"`)
		object["secret_category"] = json.RawMessage(`"ai_provider"`)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodPut, http.MethodPatch:
			var body map[string]json.RawMessage
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			if r.Method == http.MethodPatch {
				for _, field := range []string{"secret", "value"} {
					if _, ok := body[field]; ok {
						t.Errorf("metadata update resent write-only %s", field)
					}
				}
			}
			if metadata, ok := body["metadata"]; ok {
				if r.Method == http.MethodPatch {
					if kind == "ai_secret" && string(metadata) == "null" {
						break // AI-secret PATCH ignores whole-field null.
					}
					var replace []string
					_ = json.Unmarshal(body["shallow_replace_json_fields"], &replace)
					replaced := len(replace) == 1 && replace[0] == "metadata"
					if mergeOnly || (kind == "ai_secret" && !replaced) {
						metadata = mergeSecretMetadata(object["metadata"], metadata)
					}
				}
				object["metadata"] = metadata
			}
		case http.MethodGet:
		case http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
			return
		default:
			t.Errorf("unexpected method %s", r.Method)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if err := json.NewEncoder(w).Encode(object); err != nil {
			t.Error(err)
		}
	}))
	t.Cleanup(server.Close)
	return server, object
}

const (
	secretMetadataID    = "00000000-0000-4000-8000-000000000001"
	secretMetadataOrgID = "00000000-0000-4000-8000-000000000002"
)

func secretMetadataTestConfig(server *httptest.Server, kind, metadata string) string {
	return fmt.Sprintf("provider \"braintrust\" {\n api_key = \"fixture\"\n api_url = %q\n}\n%s", server.URL, secretMetadataConfig(kind, "metadata-test", secretMetadataOrgID, metadata))
}

func secretVersionAttribute(kind string) string {
	if kind == "env_var" {
		return "value_version"
	}
	return "secret_version"
}

// Exercise Terraform plans, refresh, import, replacement and clearing against
// the two API contracts, including the AI-secret opt-in replacement flag.
func TestSecretMetadataLifecycle(t *testing.T) {
	for _, kind := range []string{"ai_secret", "env_var"} {
		t.Run(kind, func(t *testing.T) {
			server, object := secretMetadataServer(t, kind, false, nil)
			config := func(metadata string) string { return secretMetadataTestConfig(server, kind, metadata) }
			rn := "braintrust_" + kind + ".test"
			initial := `{"api_base":"https://example.com","future":{"keep":1,"remove":true},"remove":"old"}`
			updated := `{"api_base":"https://example.com","future":{"keep":2,"null":null}}`
			attr := func(raw string) string { return fmt.Sprintf("metadata = %q", raw) }
			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Steps: []resource.TestStep{
					{Config: config(attr(initial)), Check: resource.TestCheckResourceAttr(rn, "metadata", initial)},
					{ResourceName: rn, ImportState: true, ImportStateVerify: true, ImportStateVerifyIgnore: []string{secretVersionAttribute(kind)}},
					{Config: config(attr(initial)), PlanOnly: true},
					{Config: config(attr(updated)), Check: resource.TestCheckResourceAttr(rn, "metadata", updated)},
					{Config: config(attr(updated)), PlanOnly: true},
					// Configured metadata still detects out-of-band changes.
					{
						PreConfig:          func() { object["metadata"] = json.RawMessage(`{"api_base":"https://changed.example.com"}`) },
						Config:             config(attr(updated)),
						PlanOnly:           true,
						ExpectNonEmptyPlan: true,
					},
					{Config: config(attr(updated)), Check: resource.TestCheckResourceAttr(rn, "metadata", updated)},
					{Config: config("metadata = jsonencode({})"), Check: resource.TestCheckResourceAttr(rn, "metadata", "{}")},
					{Config: config("metadata = jsonencode({})"), PlanOnly: true},
					// Removing the attribute stops managing metadata rather than clearing it.
					{Config: config(""), Check: resource.TestCheckResourceAttr(rn, "metadata", "{}")},
					{Config: config(""), PlanOnly: true},
				},
			})
		})
	}
}

// Resources that never configured metadata (e.g. existing configs after a
// provider upgrade) must not plan to clear metadata set in the UI, and an
// empty remote object must not produce a diff.
func TestSecretMetadataOmitted(t *testing.T) {
	for _, kind := range []string{"ai_secret", "env_var"} {
		for _, remote := range []string{"", "null", "{}"} {
			t.Run(kind+"/"+remote, func(t *testing.T) {
				var initial json.RawMessage
				if remote != "" {
					initial = json.RawMessage(remote)
				}
				server, object := secretMetadataServer(t, kind, false, initial)
				config := secretMetadataTestConfig(server, kind, "")
				rn := "braintrust_" + kind + ".test"
				ui := `{"api_base":"https://ui.example.com","additionalHeaders":{"x-z":"1","x-a":"2"}}`
				resource.UnitTest(t, resource.TestCase{
					ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
					Steps: []resource.TestStep{
						{Config: config, Check: resource.TestCheckNoResourceAttr(rn, "metadata")},
						{Config: config, PlanOnly: true},
						{PreConfig: func() { object["metadata"] = json.RawMessage(ui) }, Config: config, PlanOnly: true},
						{
							Config: config,
							Check: resource.ComposeTestCheckFunc(
								resource.TestCheckResourceAttr(rn, "metadata", `{"additionalHeaders":{"x-a":"2","x-z":"1"},"api_base":"https://ui.example.com"}`),
								func(*terraform.State) error {
									if string(object["metadata"]) != ui {
										return fmt.Errorf("UI metadata was overwritten: %s", object["metadata"])
									}
									return nil
								},
							),
						},
					},
				})
			})
		}
	}
}

// An API that merges metadata instead of replacing it must fail the apply
// with a clear error and record the server's value, not report an
// inconsistent result.
func TestSecretMetadataMergeOnlyAPI(t *testing.T) {
	for _, kind := range []string{"ai_secret", "env_var"} {
		t.Run(kind, func(t *testing.T) {
			server, _ := secretMetadataServer(t, kind, true, nil)
			config := func(metadata string) string { return secretMetadataTestConfig(server, kind, metadata) }
			rn := "braintrust_" + kind + ".test"
			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Steps: []resource.TestStep{
					{Config: config(`metadata = jsonencode({ keep = 1, remove = 2 })`)},
					{Config: config(`metadata = jsonencode({ keep = 1 })`), ExpectError: regexp.MustCompile(`Metadata Not Applied`)},
					{
						Config:             config(`metadata = jsonencode({ keep = 1 })`),
						PlanOnly:           true,
						ExpectNonEmptyPlan: true,
					},
					{RefreshState: true, ExpectNonEmptyPlan: true, Check: resource.TestCheckResourceAttr(rn, "metadata", `{"keep":1,"remove":2}`)},
				},
			})
		})
	}
}

func mergeSecretMetadata(old, update json.RawMessage) json.RawMessage {
	var previous, next map[string]json.RawMessage
	if json.Unmarshal(old, &previous) != nil || previous == nil || json.Unmarshal(update, &next) != nil || next == nil {
		return update
	}
	for key, value := range next {
		previous[key] = mergeSecretMetadata(previous[key], value)
	}
	merged, _ := json.Marshal(previous)
	return merged
}

func secretMetadataConfig(kind, name, projectID, metadata string) string {
	if kind == "ai_secret" {
		return fmt.Sprintf(`resource "braintrust_ai_secret" "test" {
 name = %q
 type = "openai"
 secret = "fixture-secret"
 secret_version = 1
 %s
}
`, name, metadata)
	}
	return fmt.Sprintf(`resource "braintrust_env_var" "test" {
 name = %q
 object_type = "project"
 object_id = %q
 secret_type = "openai"
 value = "fixture-secret"
 value_version = 1
 %s
}
`, name, projectID, metadata)
}

func TestSecretMetadataInvalidObject(t *testing.T) {
	for _, value := range []string{"[]", "null", `"string"`, "1", "{invalid"} {
		if _, err := functionJSONObject(types.StringValue(value), "metadata"); err == nil {
			t.Errorf("accepted invalid metadata %q", value)
		}
	}
}

func TestSecretMetadataEmptyState(t *testing.T) {
	for _, remote := range []string{"", "null", "{}", " { } "} {
		for _, existing := range []types.String{types.StringNull(), types.StringValue("{ }")} {
			if got := secretMetadataState(json.RawMessage(remote), existing); !got.Equal(existing) {
				t.Errorf("remote %q: got %s, want %s", remote, got, existing)
			}
		}
		if got := secretMetadataState(json.RawMessage(remote), types.StringUnknown()); !got.IsNull() {
			t.Errorf("remote %q with unknown state: got %s, want null", remote, got)
		}
		if got := secretMetadataState(json.RawMessage(remote), types.StringValue(`{"a":1}`)); !got.IsNull() {
			t.Errorf("remote %q hid removal of configured metadata: %s", remote, got)
		}
	}
	if got := secretMetadataState(json.RawMessage(`{"unexpected":true}`), types.StringNull()); got.IsNull() {
		t.Fatal("nonempty remote drift was hidden")
	}
}

func TestSecretMetadataPatch(t *testing.T) {
	state := types.StringValue(`{"a":1}`)
	for _, tc := range []struct {
		name    string
		plan    types.String
		changed bool
	}{
		{"unknown", types.StringUnknown(), false},
		{"null", types.StringNull(), false},
		{"same spelling", types.StringValue(`{"a":1.0}`), false},
		{"changed", types.StringValue(`{"a":2}`), true},
		{"cleared", types.StringValue(`{}`), true},
	} {
		metadata, changed, err := secretMetadataPatch(tc.plan, state)
		if err != nil || changed != tc.changed || (changed && string(metadata) != tc.plan.ValueString()) {
			t.Errorf("%s: metadata=%s changed=%t err=%v", tc.name, metadata, changed, err)
		}
	}
	if _, _, err := secretMetadataPatch(types.StringValue(`[]`), state); err == nil {
		t.Error("accepted non-object metadata")
	}
}

func TestAccSecretMetadata(t *testing.T) {
	for _, kind := range []string{"ai_secret", "env_var"} {
		t.Run(kind, func(t *testing.T) {
			name := "tf-acc-metadata-" + acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum)
			config := func(metadata string) string {
				if kind == "ai_secret" {
					return secretMetadataConfig(kind, name, "", metadata)
				}
				project := fmt.Sprintf("resource \"braintrust_project\" \"metadata\" { name = %q }\n", name)
				return project + strings.ReplaceAll(secretMetadataConfig(kind, name, "PROJECT_ID", metadata), `"PROJECT_ID"`, "braintrust_project.metadata.id")
			}
			rn := "braintrust_" + kind + ".test"
			version := secretVersionAttribute(kind)
			destroy := testAccCheckAISecretDestroy
			if kind == "env_var" {
				destroy = testAccCheckEnvVarDestroy
			}
			resource.Test(t, resource.TestCase{
				PreCheck:                 func() { testAccPreCheck(t) },
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				CheckDestroy:             destroy,
				Steps: []resource.TestStep{
					{Config: config(`metadata = jsonencode({ api_base = "https://example.com", additionalHeaders = { "x-keep" = "yes", "x-remove" = "old" } })`)},
					{ResourceName: rn, ImportState: true, ImportStateVerify: true, ImportStateVerifyIgnore: []string{version}},
					{Config: config(`metadata = jsonencode({ api_base = "https://example.com", additionalHeaders = { "x-keep" = "new" } })`), Check: resource.TestCheckResourceAttr(rn, "metadata", `{"additionalHeaders":{"x-keep":"new"},"api_base":"https://example.com"}`)},
					{Config: config("metadata = jsonencode({})"), Check: resource.TestCheckResourceAttr(rn, "metadata", "{}")},
					{Config: config("metadata = jsonencode({})"), PlanOnly: true},
					{Config: config(""), Check: resource.TestCheckResourceAttr(rn, "metadata", "{}")},
					{Config: config(""), PlanOnly: true},
				},
			})
		})
	}
}
