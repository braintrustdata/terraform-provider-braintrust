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
	frameworkprovider "github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

const (
	ephemeralTestAccount = "00000000-0000-0000-0002-000000000001"
	ephemeralTestToken   = "00000000-0000-0000-0001-000000000001"
	ephemeralTestOther   = "00000000-0000-0000-0001-000000000002"
	ephemeralTestSecret  = "bt-st-never-disclose-this-key"
)

func ephemeralTestConfig() serviceTokenEphemeralModel {
	return serviceTokenEphemeralModel{
		ID: types.StringUnknown(), Key: types.StringUnknown(),
		Name: types.StringValue("fixture"), ServiceAccountID: types.StringValue(ephemeralTestAccount),
		ExpiresInSeconds: types.Int64Value(3600), Nonce: types.StringValue("2026-01-01T00:00:00Z"),
	}
}

func openEphemeralTestToken(t *testing.T, r ephemeral.EphemeralResource, config serviceTokenEphemeralModel) ephemeral.OpenResponse {
	t.Helper()
	return openEphemeralTestTokenWithContext(context.Background(), t, r, config)
}

func openEphemeralTestTokenWithContext(ctx context.Context, t *testing.T, r ephemeral.EphemeralResource, config serviceTokenEphemeralModel) ephemeral.OpenResponse {
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

func TestEphemeralServiceTokenConfiguredOpenPreservesInputs(t *testing.T) {
	for _, lifetime := range []types.Int64{types.Int64Value(3600), types.Int64Null()} {
		t.Run(fmt.Sprint(lifetime), func(t *testing.T) {
			posts, lists := 0, 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if req.Header.Get("Authorization") != "Bearer fixture-owner" {
					t.Error("provider authentication was not forwarded")
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
					if body["name"] != "fixture" || body["service_account_id"] != ephemeralTestAccount {
						t.Errorf("wrong configured inputs: %v", body)
					}
					if lifetime.IsNull() {
						if _, ok := body["expires_in_seconds"]; ok {
							t.Error("omitted expiration was sent to the API")
						}
					} else if body["expires_in_seconds"] != float64(3600) {
						t.Errorf("wrong expiration: %v", body)
					}
					_, _ = fmt.Fprintf(w, `{"id":%q,"name":"normalized-by-api","service_account_id":%q,"key":%q}`, ephemeralTestToken, ephemeralTestOther, ephemeralTestSecret)
				default:
					t.Errorf("unexpected request: %s", req.Method)
					w.WriteHeader(http.StatusBadRequest)
				}
			}))
			defer server.Close()

			ctx := context.Background()
			p := &braintrustProvider{version: "test"}
			var providerSchema frameworkprovider.SchemaResponse
			p.Schema(ctx, frameworkprovider.SchemaRequest{}, &providerSchema)
			var configured frameworkprovider.ConfigureResponse
			p.Configure(ctx, frameworkprovider.ConfigureRequest{TerraformVersion: "1.11.1", Config: tfsdk.Config{
				Schema: providerSchema.Schema,
				Raw: tftypes.NewValue(providerSchema.Schema.Type().TerraformType(ctx), map[string]tftypes.Value{
					"api_key": tftypes.NewValue(tftypes.String, "fixture-owner"),
					"api_url": tftypes.NewValue(tftypes.String, server.URL),
				}),
			}}, &configured)
			if configured.Diagnostics.HasError() {
				t.Fatal(configured.Diagnostics)
			}
			if configured.EphemeralResourceData == nil || configured.EphemeralResourceData != configured.ResourceData {
				t.Fatal("ephemeral resource did not receive the configured shared client")
			}
			r := NewServiceTokenEphemeralResource()
			var resourceConfigured ephemeral.ConfigureResponse
			r.(ephemeral.EphemeralResourceWithConfigure).Configure(ctx, ephemeral.ConfigureRequest{ProviderData: configured.EphemeralResourceData}, &resourceConfigured)
			if resourceConfigured.Diagnostics.HasError() {
				t.Fatal(resourceConfigured.Diagnostics)
			}
			if _, closes := r.(ephemeral.EphemeralResourceWithClose); closes {
				t.Fatal("long-lived tokens must not be revoked on Close")
			}
			config := ephemeralTestConfig()
			config.ExpiresInSeconds = lifetime
			resp := openEphemeralTestToken(t, r, config)
			if resp.Diagnostics.HasError() {
				t.Fatal(resp.Diagnostics)
			}
			var result serviceTokenEphemeralModel
			if diags := resp.Result.Get(ctx, &result); diags.HasError() {
				t.Fatal(diags)
			}
			if !result.Name.Equal(config.Name) || !result.ServiceAccountID.Equal(config.ServiceAccountID) || !result.ExpiresInSeconds.Equal(lifetime) {
				t.Fatal("API normalization overwrote configured request fields")
			}
			if result.ID.ValueString() != ephemeralTestToken || result.Key.ValueString() != ephemeralTestSecret || posts != 1 || lists != 1 {
				t.Fatalf("missing token output or incorrect API call count: posts=%d lists=%d", posts, lists)
			}
			if resp.Private != nil || !resp.RenewAt.IsZero() {
				t.Fatal("token lifecycle must not depend on private recovery state or renewal")
			}
		})
	}
}

func TestEphemeralServiceTokenUncertainOpenPreservesConcurrentToken(t *testing.T) {
	var posts, deletes atomic.Int64
	var tokenExists atomic.Bool
	var r *serviceTokenEphemeralResource
	successfulOpen := make(chan ephemeral.OpenResponse, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case req.Method == http.MethodGet && req.URL.Path == "/v1/service_token":
			objects := []map[string]string{}
			if tokenExists.Load() {
				objects = append(objects, map[string]string{
					"id": ephemeralTestToken, "name": "fixture", "service_account_id": ephemeralTestAccount,
				})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"objects": objects})
		case req.Method == http.MethodPost && req.URL.Path == "/v1/service_token":
			if posts.Add(1) == 1 {
				// The first Open has passed preflight but commits nothing.
				// Complete another Open before returning the uncertain failure:
				// both preflight lists are empty, and the other key is delivered.
				successfulOpen <- openEphemeralTestToken(t, r, ephemeralTestConfig())
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(`{"error":"create failed"}`))
				return
			}
			tokenExists.Store(true)
			_, _ = fmt.Fprintf(w, `{"id":%q,"name":"fixture","service_account_id":%q,"key":%q}`,
				ephemeralTestToken, ephemeralTestAccount, ephemeralTestSecret)
		case req.Method == http.MethodDelete && req.URL.Path == "/v1/service_token/"+ephemeralTestToken:
			deletes.Add(1)
			tokenExists.Store(false)
			_, _ = fmt.Fprintf(w, `{"id":%q,"name":"fixture"}`, ephemeralTestToken)
		default:
			t.Errorf("unexpected request: %s %s", req.Method, req.URL.String())
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	defer server.Close()
	c, err := client.NewClient(server.URL, "fixture-owner", "")
	if err != nil {
		t.Fatal(err)
	}
	r = &serviceTokenEphemeralResource{client: c}
	failed := openEphemeralTestToken(t, r, ephemeralTestConfig())
	if !failed.Diagnostics.HasError() {
		t.Fatal("uncertain Open returned success")
	}
	var succeeded ephemeral.OpenResponse
	select {
	case succeeded = <-successfulOpen:
	default:
		t.Fatal("concurrent Open was not attempted")
	}
	if succeeded.Diagnostics.HasError() {
		t.Fatal(succeeded.Diagnostics)
	}
	var result serviceTokenEphemeralModel
	if diags := succeeded.Result.Get(context.Background(), &result); diags.HasError() {
		t.Fatal(diags)
	}
	if result.ID.ValueString() != ephemeralTestToken || result.Key.ValueString() != ephemeralTestSecret {
		t.Fatal("concurrent Open did not deliver its token")
	}
	if posts.Load() != 2 || deletes.Load() != 0 || !tokenExists.Load() {
		t.Fatalf("uncertain Open revoked another caller's token: posts=%d deletes=%d exists=%t",
			posts.Load(), deletes.Load(), tokenExists.Load())
	}
	var details strings.Builder
	for _, diagnostic := range failed.Diagnostics {
		details.WriteString(diagnostic.Detail())
	}
	if !strings.Contains(details.String(), ephemeralTestToken) {
		t.Fatal("uncertain Open did not identify the candidate for manual inspection")
	}
	if strings.Contains(details.String(), ephemeralTestSecret) {
		t.Fatal("uncertain Open disclosed the other caller's key")
	}
}

func TestEphemeralServiceTokenCreationFailures(t *testing.T) {
	token := func(id, account string) map[string]string {
		return map[string]string{"id": id, "name": "fixture", "service_account_id": account}
	}
	newToken := token(ephemeralTestToken, ephemeralTestAccount)
	otherToken := token(ephemeralTestOther, ephemeralTestAccount)
	unrelatedToken := token(ephemeralTestOther, ephemeralTestOther)
	for _, tc := range []struct {
		name          string
		before, after []map[string]string
		postStatus    int
		postBody      string
		listStatus    int
		deleteStatus  int
		wantPosts     int
		wantDeletes   int
		wantDetail    []string
	}{
		{
			name: "duplicate refused without adoption", before: []map[string]string{newToken},
			wantDetail: []string{ephemeralTestToken},
		},
		{
			name: "undecoded success requires inspection", postStatus: http.StatusCreated,
			postBody: `{"key":"` + ephemeralTestSecret + `"}`, after: []map[string]string{newToken},
			wantPosts: 1, wantDetail: []string{ephemeralTestToken},
		},
		{
			name: "missing key cleaned", postBody: `{"id":"` + ephemeralTestToken + `"}`,
			after: []map[string]string{newToken}, wantPosts: 1, wantDeletes: 1,
			wantDetail: []string{ephemeralTestToken},
		},
		{
			name: "missing id requires inspection", postBody: `{"key":"` + ephemeralTestSecret + `"}`,
			after: []map[string]string{newToken}, wantPosts: 1,
			wantDetail: []string{ephemeralTestToken},
		},
		{
			name: "malformed response requires inspection", postBody: `{"key":"` + ephemeralTestSecret + `","id":false}`,
			after: []map[string]string{newToken}, wantPosts: 1,
			wantDetail: []string{ephemeralTestToken},
		},
		{
			name: "server failure not retried", postStatus: http.StatusInternalServerError, postBody: ephemeralTestSecret,
			after: []map[string]string{newToken}, wantPosts: 1,
			wantDetail: []string{ephemeralTestToken},
		},
		{
			name: "cleanup fails safely", postBody: `{"id":"` + ephemeralTestToken + `"}`, after: []map[string]string{newToken},
			deleteStatus: http.StatusForbidden, wantPosts: 1, wantDeletes: 1,
			wantDetail: []string{ephemeralTestToken},
		},
		{
			name: "cleanup uncertain not retried", postBody: `{"id":"` + ephemeralTestToken + `"}`, after: []map[string]string{newToken},
			deleteStatus: http.StatusInternalServerError, wantPosts: 1, wantDeletes: 1,
			wantDetail: []string{ephemeralTestToken},
		},
		{
			name: "cleanup already absent", postBody: `{"id":"` + ephemeralTestToken + `"}`, after: []map[string]string{newToken},
			deleteStatus: http.StatusNotFound, wantPosts: 1, wantDeletes: 1,
			wantDetail: []string{ephemeralTestToken},
		},
		{
			name: "ambiguous candidates never deleted", postBody: `{}`, after: []map[string]string{newToken, otherToken},
			wantPosts: 1, wantDetail: []string{ephemeralTestToken, ephemeralTestOther},
		},
		{
			name: "known response id disagrees with candidate", postBody: `{"id":"` + ephemeralTestOther + `"}`,
			after: []map[string]string{newToken}, wantPosts: 1,
			wantDetail: []string{ephemeralTestToken, ephemeralTestOther},
		},
		{
			name: "no visible candidate", postBody: `{}`, wantPosts: 1,
		},
		{
			name: "reconciliation list fails", postBody: `{"id":"` + ephemeralTestToken + `"}`,
			listStatus: http.StatusForbidden, wantPosts: 1,
			wantDetail: []string{ephemeralTestToken},
		},
		{
			name: "preexisting id never deleted even if account changes", postBody: `{}`,
			before: []map[string]string{unrelatedToken}, after: []map[string]string{otherToken}, wantPosts: 1,
		},
		{
			name: "preexisting other account preserved", postBody: `{"id":"` + ephemeralTestToken + `"}`,
			before: []map[string]string{unrelatedToken}, after: []map[string]string{unrelatedToken, newToken},
			wantPosts: 1, wantDeletes: 1, wantDetail: []string{ephemeralTestToken},
		},
		{
			name: "definite failure sanitized", postStatus: http.StatusBadRequest, postBody: ephemeralTestSecret,
			wantPosts: 1,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			posts, lists, deletes := 0, 0, 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch {
				case req.Method == http.MethodGet && req.URL.Path == "/v1/service_token":
					lists++
					if req.URL.Query().Get("name") != "fixture" {
						t.Error("list did not filter by configured name")
					}
					objects := tc.before
					if posts > 0 {
						objects = tc.after
						if tc.listStatus != 0 {
							w.WriteHeader(tc.listStatus)
							_, _ = w.Write([]byte(ephemeralTestSecret))
							return
						}
					}
					if objects == nil {
						objects = []map[string]string{}
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"objects": objects})
				case req.Method == http.MethodPost && req.URL.Path == "/v1/service_token":
					posts++
					if tc.postStatus != 0 {
						w.WriteHeader(tc.postStatus)
					}
					_, _ = w.Write([]byte(tc.postBody))
				case req.Method == http.MethodDelete:
					deletes++
					if req.URL.Path != "/v1/service_token/"+ephemeralTestToken {
						t.Errorf("deleted a token other than the uniquely new token: %s", req.URL.Path)
					}
					if tc.deleteStatus != 0 {
						w.WriteHeader(tc.deleteStatus)
						_, _ = w.Write([]byte(ephemeralTestSecret))
					} else {
						_, _ = fmt.Fprintf(w, `{"id":%q,"name":"fixture"}`, ephemeralTestToken)
					}
				default:
					t.Errorf("unexpected request: %s %s", req.Method, req.URL.String())
					w.WriteHeader(http.StatusBadRequest)
				}
			}))
			defer server.Close()
			c, err := client.NewClient(server.URL, "fixture-owner", "")
			if err != nil {
				t.Fatal(err)
			}
			resp := openEphemeralTestToken(t, &serviceTokenEphemeralResource{client: c}, ephemeralTestConfig())
			if !resp.Diagnostics.HasError() {
				t.Fatal("creation failure returned success")
			}
			var details strings.Builder
			for _, d := range resp.Diagnostics {
				details.WriteString(d.Summary() + " " + d.Detail() + "\n")
			}
			if strings.Contains(details.String(), ephemeralTestSecret) {
				t.Fatal("diagnostics disclosed a raw key or response body")
			}
			for _, want := range tc.wantDetail {
				if !strings.Contains(details.String(), want) {
					t.Errorf("diagnostic missing %q: %s", want, details.String())
				}
			}
			if posts != tc.wantPosts || deletes != tc.wantDeletes {
				t.Errorf("posts=%d deletes=%d, want posts=%d deletes=%d", posts, deletes, tc.wantPosts, tc.wantDeletes)
			}
			var result serviceTokenEphemeralModel
			if diags := resp.Result.Get(context.Background(), &result); diags.HasError() {
				t.Fatal(diags)
			}
			if !result.ID.IsUnknown() || !result.Key.IsUnknown() || resp.Private != nil {
				t.Fatal("failed Open exposed output or retained private recovery state")
			}
		})
	}
}

// Interrupted Opens must reconcile with an uncanceled context, but cannot revoke
// a candidate without a response ID attributing it to this attempt.
func TestEphemeralServiceTokenCanceledOpenReportsCandidate(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	posts, deletes := 0, 0
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case req.Method == http.MethodGet && req.URL.Path == "/v1/service_token":
			objects := []map[string]string{}
			if posts > 0 {
				objects = append(objects, map[string]string{"id": ephemeralTestToken, "name": "fixture", "service_account_id": ephemeralTestAccount})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"objects": objects})
		case req.Method == http.MethodPost && req.URL.Path == "/v1/service_token":
			// The server commits the token, then the caller gives up before the response.
			posts++
			cancel()
			select {
			case <-req.Context().Done():
			case <-release:
			}
		case req.Method == http.MethodDelete && req.URL.Path == "/v1/service_token/"+ephemeralTestToken:
			deletes++
			_, _ = fmt.Fprintf(w, `{"id":%q,"name":"fixture"}`, ephemeralTestToken)
		default:
			t.Errorf("unexpected request: %s %s", req.Method, req.URL.String())
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	defer server.Close()
	defer close(release)
	c, err := client.NewClient(server.URL, "fixture-owner", "")
	if err != nil {
		t.Fatal(err)
	}
	resp := openEphemeralTestTokenWithContext(ctx, t, &serviceTokenEphemeralResource{client: c}, ephemeralTestConfig())
	if !resp.Diagnostics.HasError() {
		t.Fatal("canceled Open returned success")
	}
	if posts != 1 || deletes != 0 {
		t.Fatalf("posts=%d deletes=%d, want no revocation without a response ID: %v", posts, deletes, resp.Diagnostics)
	}
	var details strings.Builder
	for _, diagnostic := range resp.Diagnostics {
		details.WriteString(diagnostic.Detail())
	}
	if !strings.Contains(details.String(), ephemeralTestToken) {
		t.Fatal("canceled Open did not reconcile the token for manual inspection")
	}
}

func TestEphemeralServiceTokenRejectsInvalidOpenInputs(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer server.Close()
	c, err := client.NewClient(server.URL, "fixture-owner", "")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		change func(*serviceTokenEphemeralModel)
	}{
		{"unknown name", func(c *serviceTokenEphemeralModel) { c.Name = types.StringUnknown() }},
		{"missing name", func(c *serviceTokenEphemeralModel) { c.Name = types.StringNull() }},
		{"empty name", func(c *serviceTokenEphemeralModel) { c.Name = types.StringValue("") }},
		{"unknown account", func(c *serviceTokenEphemeralModel) { c.ServiceAccountID = types.StringUnknown() }},
		{"empty account", func(c *serviceTokenEphemeralModel) { c.ServiceAccountID = types.StringValue("") }},
		{"missing nonce", func(c *serviceTokenEphemeralModel) { c.Nonce = types.StringNull() }},
		{"empty nonce", func(c *serviceTokenEphemeralModel) { c.Nonce = types.StringValue("") }},
		{"unknown lifetime", func(c *serviceTokenEphemeralModel) { c.ExpiresInSeconds = types.Int64Unknown() }},
		{"zero lifetime", func(c *serviceTokenEphemeralModel) { c.ExpiresInSeconds = types.Int64Value(0) }},
		{"negative lifetime", func(c *serviceTokenEphemeralModel) { c.ExpiresInSeconds = types.Int64Value(-1) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			config := ephemeralTestConfig()
			tc.change(&config)
			resp := openEphemeralTestToken(t, &serviceTokenEphemeralResource{client: c}, config)
			if !resp.Diagnostics.HasError() {
				t.Fatal("invalid Open input accepted")
			}
		})
	}
	if requests != 0 {
		t.Fatalf("invalid configuration made %d API calls", requests)
	}
	resp := openEphemeralTestToken(t, &serviceTokenEphemeralResource{}, ephemeralTestConfig())
	if !resp.Diagnostics.HasError() {
		t.Fatal("Open accepted a missing provider client")
	}
}
