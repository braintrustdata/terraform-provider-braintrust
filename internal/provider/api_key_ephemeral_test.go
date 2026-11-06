package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/braintrustdata/terraform-provider-braintrust/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/ephemeral"
	"github.com/hashicorp/terraform-plugin-framework/ephemeral/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

const (
	apiKeyEphemeralTestOrg    = "00000000-0000-0000-0002-000000000001"
	apiKeyEphemeralTestID     = "00000000-0000-0000-0001-000000000001"
	apiKeyEphemeralTestOther  = "00000000-0000-0000-0001-000000000002"
	apiKeyEphemeralTestSecret = "bt-api-never-disclose-this-key"
)

func apiKeyEphemeralTestConfig() apiKeyEphemeralModel {
	return apiKeyEphemeralModel{
		ID: types.StringUnknown(), Key: types.StringUnknown(),
		Name: types.StringValue("fixture"), OrgID: types.StringValue(apiKeyEphemeralTestOrg),
		ExpiresInSeconds: types.Int64Value(3600), Nonce: types.StringValue("2026-01-01T00:00:00Z"),
	}
}

func openEphemeralTestAPIKey(ctx context.Context, t *testing.T, r ephemeral.EphemeralResource, config apiKeyEphemeralModel) ephemeral.OpenResponse {
	t.Helper()
	var schemaResp ephemeral.SchemaResponse
	r.Schema(ctx, ephemeral.SchemaRequest{}, &schemaResp)
	data := tfsdk.EphemeralResultData{Schema: schemaResp.Schema}
	if diags := data.Set(ctx, config); diags.HasError() {
		t.Fatal(diags)
	}
	resp := ephemeral.OpenResponse{Result: data}
	r.Open(ctx, ephemeral.OpenRequest{Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: data.Raw}}, &resp)
	return resp
}

func apiKeyEphemeralTestClient(t *testing.T, url string) *client.Client {
	t.Helper()
	c, err := client.NewClient(url, "fixture-user-key", "")
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestEphemeralAPIKeyOpenAndStateFreeContract(t *testing.T) {
	for _, omitted := range []bool{false, true} {
		t.Run(fmt.Sprint(omitted), func(t *testing.T) {
			posts, lists := 0, 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if req.Header.Get("Authorization") != "Bearer fixture-user-key" {
					t.Error("caller authentication was not forwarded")
				}
				switch req.Method {
				case http.MethodGet:
					lists++
					_, _ = w.Write([]byte(`{"objects":[]}`))
				case http.MethodPost:
					posts++
					var body map[string]any
					if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
						t.Error(err)
					}
					if body["name"] != "fixture" {
						t.Error("wrong requested name")
					}
					if _, ok := body["nonce"]; ok {
						t.Error("nonce was sent to the API")
					}
					if omitted {
						if _, ok := body["org_id"]; ok {
							t.Error("omitted organization was sent")
						}
						if _, ok := body["expires_in_seconds"]; ok {
							t.Error("omitted lifetime was sent")
						}
					} else if body["org_id"] != apiKeyEphemeralTestOrg || body["expires_in_seconds"] != float64(3600) {
						t.Error("wrong requested organization or lifetime")
					}
					_, _ = fmt.Fprintf(w, `{"id":%q,"name":"fixture","org_id":%q,"key":%q}`, apiKeyEphemeralTestID, apiKeyEphemeralTestOrg, apiKeyEphemeralTestSecret)
				default:
					t.Errorf("unexpected method: %s", req.Method)
					w.WriteHeader(http.StatusBadRequest)
				}
			}))
			defer server.Close()
			r := NewAPIKeyEphemeralResource()
			var configured ephemeral.ConfigureResponse
			r.(ephemeral.EphemeralResourceWithConfigure).Configure(context.Background(), ephemeral.ConfigureRequest{ProviderData: apiKeyEphemeralTestClient(t, server.URL)}, &configured)
			if configured.Diagnostics.HasError() {
				t.Fatal(configured.Diagnostics)
			}
			var metadata ephemeral.MetadataResponse
			r.Metadata(context.Background(), ephemeral.MetadataRequest{ProviderTypeName: "braintrust"}, &metadata)
			if metadata.TypeName != "braintrust_api_key" {
				t.Fatal("wrong ephemeral type name")
			}
			if _, closes := r.(ephemeral.EphemeralResourceWithClose); closes {
				t.Fatal("delivered keys must not be revoked on Close")
			}
			var schemaResp ephemeral.SchemaResponse
			r.Schema(context.Background(), ephemeral.SchemaRequest{}, &schemaResp)
			keyAttr := schemaResp.Schema.Attributes["key"].(schema.StringAttribute)
			if !keyAttr.Computed || !keyAttr.Sensitive {
				t.Fatal("raw key must be computed and sensitive")
			}
			config := apiKeyEphemeralTestConfig()
			if omitted {
				config.OrgID = types.StringNull()
				config.ExpiresInSeconds = types.Int64Null()
			}
			resp := openEphemeralTestAPIKey(context.Background(), t, r, config)
			if resp.Diagnostics.HasError() {
				t.Fatal(resp.Diagnostics)
			}
			var result apiKeyEphemeralModel
			idAttr := schemaResp.Schema.Attributes["id"].(schema.StringAttribute)
			if !idAttr.Computed || !idAttr.Sensitive {
				t.Fatal("key ID must be computed and sensitive")
			}
			if diags := resp.Result.Get(context.Background(), &result); diags.HasError() {
				t.Fatal(diags)
			}
			if result.ID.ValueString() != apiKeyEphemeralTestID || result.Key.ValueString() != apiKeyEphemeralTestSecret {
				t.Fatal("created key was not delivered")
			}
			if !result.OrgID.Equal(config.OrgID) || !result.ExpiresInSeconds.Equal(config.ExpiresInSeconds) || !result.Nonce.Equal(config.Nonce) {
				t.Fatal("configured inputs changed")
			}
			if posts != 1 || lists != 1 || resp.Private != nil || !resp.RenewAt.IsZero() {
				t.Fatal("Open must create once without renewal or private recovery state")
			}
		})
	}
}

func TestEphemeralAPIKeyUnknownAndInvalidInputs(t *testing.T) {
	for name, mutate := range map[string]func(*apiKeyEphemeralModel){
		"unknown name":         func(m *apiKeyEphemeralModel) { m.Name = types.StringUnknown() },
		"null name":            func(m *apiKeyEphemeralModel) { m.Name = types.StringNull() },
		"unknown nonce":        func(m *apiKeyEphemeralModel) { m.Nonce = types.StringUnknown() },
		"empty nonce":          func(m *apiKeyEphemeralModel) { m.Nonce = types.StringValue("") },
		"unknown organization": func(m *apiKeyEphemeralModel) { m.OrgID = types.StringUnknown() },
		"empty organization":   func(m *apiKeyEphemeralModel) { m.OrgID = types.StringValue("") },
		"unknown lifetime":     func(m *apiKeyEphemeralModel) { m.ExpiresInSeconds = types.Int64Unknown() },
		"zero lifetime":        func(m *apiKeyEphemeralModel) { m.ExpiresInSeconds = types.Int64Value(0) },
		"negative lifetime":    func(m *apiKeyEphemeralModel) { m.ExpiresInSeconds = types.Int64Value(-1) },
	} {
		t.Run(name, func(t *testing.T) {
			var calls atomic.Int64
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				calls.Add(1)
				w.WriteHeader(http.StatusBadRequest)
			}))
			defer server.Close()
			config := apiKeyEphemeralTestConfig()
			mutate(&config)
			r := &apiKeyEphemeralResource{client: apiKeyEphemeralTestClient(t, server.URL)}
			resp := openEphemeralTestAPIKey(context.Background(), t, r, config)
			if !resp.Diagnostics.HasError() || calls.Load() != 0 {
				t.Fatal("invalid or unknown inputs must fail before any API request")
			}
		})
	}
}

func TestEphemeralAPIKeyUncertainCleanupOwnership(t *testing.T) {
	key := func(id, org string) map[string]string {
		return map[string]string{"id": id, "name": "fixture", "org_id": org, "user_id": apiKeyEphemeralTestOther}
	}
	newKey := key(apiKeyEphemeralTestID, apiKeyEphemeralTestOrg)
	otherKey := key(apiKeyEphemeralTestOther, apiKeyEphemeralTestOrg)
	partial := fmt.Sprintf(`{"id":%q,"name":"fixture","org_id":%q}`, apiKeyEphemeralTestID, apiKeyEphemeralTestOrg)
	for _, tc := range []struct {
		name                             string
		before, after                    []map[string]string
		body                             string
		status, listStatus, deleteStatus int
		omitOrg                          bool
		wantPosts, wantDeletes           int
		wantDetail                       []string
	}{
		{name: "preexisting refused", before: []map[string]string{newKey}, wantDetail: []string{apiKeyEphemeralTestID}},
		{name: "omitted org rejects any visible scope", before: []map[string]string{key(apiKeyEphemeralTestID, apiKeyEphemeralTestOther)}, omitOrg: true, wantDetail: []string{apiKeyEphemeralTestID}},
		{name: "response identified sole new key", body: partial, after: []map[string]string{newKey}, wantPosts: 1, wantDeletes: 1, wantDetail: []string{"revoked or is already absent"}},
		{name: "lost response cannot claim concurrent key", status: 500, body: apiKeyEphemeralTestSecret, after: []map[string]string{newKey}, wantPosts: 1, wantDetail: []string{"ambiguous", apiKeyEphemeralTestID}},
		{name: "multiple new candidates not deleted", body: partial, after: []map[string]string{newKey, otherKey}, wantPosts: 1, wantDetail: []string{"ambiguous", apiKeyEphemeralTestID, apiKeyEphemeralTestOther}},
		{name: "preexisting response ID not deleted", before: []map[string]string{key(apiKeyEphemeralTestID, apiKeyEphemeralTestOther)}, body: partial, after: []map[string]string{newKey}, wantPosts: 1, wantDetail: []string{"No new matching key"}},
		{name: "successful preexisting response not adopted", before: []map[string]string{key(apiKeyEphemeralTestID, apiKeyEphemeralTestOther)}, body: fmt.Sprintf(`{"id":%q,"key":%q}`, apiKeyEphemeralTestID, apiKeyEphemeralTestSecret), wantPosts: 1, wantDetail: []string{"not adopted or deleted"}},
		{name: "wrong organization not deleted", body: partial, after: []map[string]string{key(apiKeyEphemeralTestID, apiKeyEphemeralTestOther)}, wantPosts: 1, wantDetail: []string{"No new matching key"}},
		{name: "response user mismatch not deleted", body: fmt.Sprintf(`{"id":%q,"user_id":%q}`, apiKeyEphemeralTestID, apiKeyEphemeralTestID), after: []map[string]string{newKey}, wantPosts: 1, wantDetail: []string{"ambiguous"}},
		{name: "response organization mismatch not deleted", body: fmt.Sprintf(`{"id":%q,"org_id":%q}`, apiKeyEphemeralTestID, apiKeyEphemeralTestOther), after: []map[string]string{newKey}, wantPosts: 1, wantDetail: []string{"ambiguous"}},
		{name: "malformed ID cannot leak or delete", body: fmt.Sprintf(`{"id":%q,"key":%q}`, apiKeyEphemeralTestSecret, apiKeyEphemeralTestSecret), after: []map[string]string{newKey}, wantPosts: 1, wantDetail: []string{"ambiguous"}},
		{name: "wrong type cannot leak", body: fmt.Sprintf(`{"key":%q,"id":{}}`, apiKeyEphemeralTestSecret), after: []map[string]string{newKey}, wantPosts: 1, wantDetail: []string{"ambiguous"}},
		{name: "response name mismatch not deleted", body: fmt.Sprintf(`{"id":%q,"name":%q}`, apiKeyEphemeralTestID, apiKeyEphemeralTestSecret), after: []map[string]string{newKey}, wantPosts: 1, wantDetail: []string{"ambiguous"}},
		{name: "list errors redact response", listStatus: 500, body: apiKeyEphemeralTestSecret, wantDetail: []string{"No key was created"}},
		{name: "delete errors redact response", body: partial, after: []map[string]string{newKey}, deleteStatus: 500, wantPosts: 1, wantDeletes: 1, wantDetail: []string{"Inspect and revoke this ID manually"}},
		{name: "explicit org ignores other org", before: []map[string]string{key(apiKeyEphemeralTestOther, apiKeyEphemeralTestOther)}, body: partial, after: []map[string]string{newKey}, wantPosts: 1, wantDeletes: 1, wantDetail: []string{"revoked or is already absent"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var posts, deletes atomic.Int64
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch req.Method {
				case http.MethodGet:
					if tc.listStatus != 0 {
						w.WriteHeader(tc.listStatus)
						_, _ = w.Write([]byte(apiKeyEphemeralTestSecret))
						return
					}
					objects := tc.before
					if posts.Load() > 0 {
						objects = tc.after
					}
					if objects == nil {
						objects = []map[string]string{}
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"objects": objects})
				case http.MethodPost:
					posts.Add(1)
					if tc.status != 0 {
						w.WriteHeader(tc.status)
					}
					_, _ = w.Write([]byte(tc.body))
				case http.MethodDelete:
					deletes.Add(1)
					if req.URL.Path != "/v1/api_key/"+apiKeyEphemeralTestID {
						t.Error("cleanup deleted another key")
					}
					if tc.deleteStatus != 0 {
						w.WriteHeader(tc.deleteStatus)
						_, _ = w.Write([]byte(apiKeyEphemeralTestSecret))
					} else {
						_, _ = fmt.Fprintf(w, `{"id":%q}`, apiKeyEphemeralTestID)
					}
				default:
					t.Errorf("unexpected method: %s", req.Method)
				}
			}))
			defer server.Close()
			r := &apiKeyEphemeralResource{client: apiKeyEphemeralTestClient(t, server.URL)}
			config := apiKeyEphemeralTestConfig()
			if tc.omitOrg {
				config.OrgID = types.StringNull()
			}
			resp := openEphemeralTestAPIKey(context.Background(), t, r, config)
			if !resp.Diagnostics.HasError() {
				t.Fatal("expected a creation error")
			}
			if posts.Load() != int64(tc.wantPosts) || deletes.Load() != int64(tc.wantDeletes) {
				t.Fatalf("posts=%d deletes=%d, want posts=%d deletes=%d", posts.Load(), deletes.Load(), tc.wantPosts, tc.wantDeletes)
			}
			var details strings.Builder
			for _, diagnostic := range resp.Diagnostics {
				details.WriteString(diagnostic.Summary())
				details.WriteString(diagnostic.Detail())
			}
			if strings.Contains(details.String(), apiKeyEphemeralTestSecret) {
				t.Fatal("diagnostics disclosed a credential")
			}
			for _, expected := range tc.wantDetail {
				if !strings.Contains(details.String(), expected) {
					t.Errorf("missing diagnostic detail %q", expected)
				}
			}
			if resp.Private != nil {
				t.Fatal("failed Open must not persist recovery state")
			}
		})
	}
}

func TestEphemeralAPIKeyCanceledOpenReconciles(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var posts, lists, deletes atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch req.Method {
		case http.MethodGet:
			lists.Add(1)
			if posts.Load() == 0 {
				_, _ = w.Write([]byte(`{"objects":[]}`))
			} else {
				_, _ = fmt.Fprintf(w, `{"objects":[{"id":%q,"name":"fixture","org_id":%q}]}`, apiKeyEphemeralTestID, apiKeyEphemeralTestOrg)
			}
		case http.MethodPost:
			posts.Add(1)
			cancel()
		case http.MethodDelete:
			deletes.Add(1)
		default:
			t.Errorf("unexpected request: %s", req.Method)
		}
	}))
	defer server.Close()
	r := &apiKeyEphemeralResource{client: apiKeyEphemeralTestClient(t, server.URL)}
	resp := openEphemeralTestAPIKey(ctx, t, r, apiKeyEphemeralTestConfig())
	if !resp.Diagnostics.HasError() || posts.Load() != 1 || lists.Load() != 2 || deletes.Load() != 0 {
		t.Fatalf("canceled Open must reconcile without retrying or deleting unattributed keys: posts=%d lists=%d deletes=%d", posts.Load(), lists.Load(), deletes.Load())
	}
}
