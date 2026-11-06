package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/braintrustdata/terraform-provider-braintrust/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

const (
	apiKeyTestID     = "00000000-0000-0000-0000-000000000001"
	apiKeyTestOrg    = "00000000-0000-0000-0000-000000000002"
	apiKeyTestUser   = "00000000-0000-0000-0000-000000000003"
	apiKeyTestOther  = "00000000-0000-0000-0000-000000000004"
	apiKeyTestSecret = "bt-never-disclose-raw-key"
)

func apiKeyTestObject(id, name, org string) string {
	return fmt.Sprintf(`{"id":%q,"name":%q,"org_id":%q,"user_id":%q,"preview_name":"bt-...test","created":"2026-01-01T00:00:00Z","expires_at":"2026-01-01T00:30:00Z","user_email":"user@example.com","user_given_name":"Test","user_family_name":"User"}`, id, name, org, apiKeyTestUser)
}

func newAPIKeyTestResource(t *testing.T, handler http.HandlerFunc) *apiKeyResource {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	c, err := client.NewClient(server.URL, "fixture-owner", "")
	if err != nil {
		t.Fatal(err)
	}
	return &apiKeyResource{client: c}
}

func apiKeyTestState(t *testing.T, r *apiKeyResource, model apiKeyModel) tfsdk.State {
	t.Helper()
	ctx := context.Background()
	var s resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &s)
	state := tfsdk.State{Schema: s.Schema}
	if diags := state.Set(ctx, model); diags.HasError() {
		t.Fatal(diags)
	}
	return state
}

func createAPIKeyTest(t *testing.T, r *apiKeyResource, model apiKeyModel) resource.CreateResponse {
	t.Helper()
	state := apiKeyTestState(t, r, model)
	resp := resource.CreateResponse{State: tfsdk.State{Schema: state.Schema, Raw: tftypes.NewValue(state.Schema.Type().TerraformType(context.Background()), nil)}}
	r.Create(context.Background(), resource.CreateRequest{Plan: tfsdk.Plan(state)}, &resp)
	return resp
}

func apiKeyTestPlan() apiKeyModel {
	return apiKeyModel{Name: types.StringValue("fixture"), OrgID: types.StringUnknown(), Key: types.StringUnknown(), ExpiresInSeconds: types.Int64Value(3600)}
}

func TestAPIKeyCreateRefreshAndImport(t *testing.T) {
	posts := 0
	r := newAPIKeyTestResource(t, func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case req.Method == http.MethodGet && req.URL.Path == "/v1/api_key":
			if req.URL.Query().Get("api_key_name") != "fixture" {
				t.Error("missing SDK name filter")
			}
			_, _ = w.Write([]byte(`{"objects":[]}`))
		case req.Method == http.MethodPost && req.URL.Path == "/v1/api_key":
			posts++
			var body map[string]json.RawMessage
			if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			if _, ok := body["org_id"]; ok {
				t.Error("computed unknown organization must be omitted")
			}
			if string(body["expires_in_seconds"]) != "3600" {
				t.Errorf("wrong lifetime: %s", body["expires_in_seconds"])
			}
			object := apiKeyTestObject(apiKeyTestID, "fixture", apiKeyTestOrg)
			_, _ = w.Write([]byte(strings.TrimSuffix(object, "}") + `,"key":"` + apiKeyTestSecret + `"}`))
		case req.Method == http.MethodGet && req.URL.Path == "/v1/api_key/"+apiKeyTestID:
			_, _ = w.Write([]byte(apiKeyTestObject(apiKeyTestID, "fixture", apiKeyTestOrg)))
		default:
			t.Errorf("unexpected request: %s %s", req.Method, req.URL)
			w.WriteHeader(http.StatusNotFound)
		}
	})
	created := createAPIKeyTest(t, r, apiKeyTestPlan())
	if created.Diagnostics.HasError() {
		t.Fatal(created.Diagnostics)
	}
	read := resource.ReadResponse{State: created.State}
	r.Read(context.Background(), resource.ReadRequest{State: created.State}, &read)
	if read.Diagnostics.HasError() {
		t.Fatal(read.Diagnostics)
	}
	var model apiKeyModel
	if diags := read.State.Get(context.Background(), &model); diags.HasError() {
		t.Fatal(diags)
	}
	if model.Key.ValueString() != apiKeyTestSecret || model.ExpiresInSeconds.ValueInt64() != 3600 || model.OrgID.ValueString() != apiKeyTestOrg {
		t.Fatal("refresh lost secret, requested lifetime, or default organization")
	}
	imported := resource.ImportStateResponse{State: created.State}
	r.ImportState(context.Background(), resource.ImportStateRequest{ID: apiKeyTestID}, &imported)
	if imported.Diagnostics.HasError() {
		t.Fatal(imported.Diagnostics)
	}
	read = resource.ReadResponse{State: imported.State}
	r.Read(context.Background(), resource.ReadRequest{State: imported.State}, &read)
	if read.Diagnostics.HasError() {
		t.Fatal(read.Diagnostics)
	}
	if diags := read.State.Get(context.Background(), &model); diags.HasError() {
		t.Fatal(diags)
	}
	if !model.Key.IsNull() || !model.ExpiresInSeconds.IsNull() || model.Name.ValueString() != "fixture" || posts != 1 {
		t.Fatal("import must populate metadata without synthesizing secret/lifetime or minting a key")
	}
}

func TestAPIKeyCreateLifetimeAndExplicitOrganization(t *testing.T) {
	for _, lifetime := range []types.Int64{types.Int64Null(), types.Int64Value(1), types.Int64Value(math.MaxInt64), types.Int64Value(0), types.Int64Value(-1)} {
		t.Run(lifetime.String(), func(t *testing.T) {
			posts := 0
			r := newAPIKeyTestResource(t, func(w http.ResponseWriter, req *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if req.Method == http.MethodGet {
					_, _ = w.Write([]byte(`{"objects":[]}`))
					return
				}
				posts++
				var body struct {
					OrgID   string `json:"org_id"`
					Seconds *int64 `json:"expires_in_seconds"`
				}
				if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				if body.OrgID != apiKeyTestOrg {
					t.Error("explicit organization was lost")
				}
				if lifetime.IsNull() {
					if body.Seconds != nil {
						t.Error("omitted lifetime was sent")
					}
				} else if body.Seconds == nil || *body.Seconds != lifetime.ValueInt64() {
					t.Error("lifetime overflowed or changed")
				}
				_, _ = fmt.Fprintf(w, `{"id":%q,"name":"fixture","org_id":%q,"key":%q}`, apiKeyTestID, apiKeyTestOrg, apiKeyTestSecret)
			})
			plan := apiKeyTestPlan()
			plan.OrgID, plan.ExpiresInSeconds = types.StringValue(apiKeyTestOrg), lifetime
			resp := createAPIKeyTest(t, r, plan)
			invalid := !lifetime.IsNull() && lifetime.ValueInt64() <= 0
			if resp.Diagnostics.HasError() != invalid {
				t.Fatal(resp.Diagnostics)
			}
			if invalid && posts != 0 || !invalid && posts != 1 {
				t.Fatalf("unexpected POST count: %d", posts)
			}
		})
	}
}

func TestAPIKeyPreexistingAndUncertainCreateSafety(t *testing.T) {
	cases := []struct {
		name, before, response, recovered string
		status                            int
		wantPost, wantState, explicitOrg  bool
	}{
		{name: "preexisting name", before: apiKeyTestObject(apiKeyTestID, "fixture", apiKeyTestOrg)},
		{name: "preexisting response ID", before: apiKeyTestObject(apiKeyTestID, "different", apiKeyTestOrg), response: `{"id":"` + apiKeyTestID + `","name":"fixture","key":"` + apiKeyTestSecret + `"}`, wantPost: true},
		{name: "lost response concurrent key", response: `{"error":"` + apiKeyTestSecret + `"}`, status: 500, recovered: apiKeyTestObject(apiKeyTestOther, "fixture", apiKeyTestOrg), wantPost: true},
		{name: "exact response ID missing secret", response: apiKeyTestObject(apiKeyTestID, "fixture", apiKeyTestOrg), recovered: apiKeyTestObject(apiKeyTestID, "fixture", apiKeyTestOrg), wantPost: true, wantState: true},
		{name: "exact ID wrong owner", response: strings.ReplaceAll(apiKeyTestObject(apiKeyTestID, "fixture", apiKeyTestOrg), apiKeyTestUser, apiKeyTestOther), recovered: apiKeyTestObject(apiKeyTestID, "fixture", apiKeyTestOrg), wantPost: true},
		{name: "exact ID wrong name", response: apiKeyTestObject(apiKeyTestID, "fixture", apiKeyTestOrg), recovered: apiKeyTestObject(apiKeyTestID, "other", apiKeyTestOrg), wantPost: true},
		{name: "exact ID wrong org", response: apiKeyTestObject(apiKeyTestID, "fixture", apiKeyTestOrg), recovered: apiKeyTestObject(apiKeyTestID, "fixture", apiKeyTestOther), wantPost: true},
		{name: "missing ownership metadata", response: `{"id":"` + apiKeyTestID + `","name":"fixture"}`, recovered: `{"id":"` + apiKeyTestID + `","name":"fixture"}`, wantPost: true},
		{name: "uncertain preexisting response", before: apiKeyTestObject(apiKeyTestID, "different", apiKeyTestOrg), response: apiKeyTestObject(apiKeyTestID, "fixture", apiKeyTestOrg), recovered: apiKeyTestObject(apiKeyTestID, "fixture", apiKeyTestOrg), wantPost: true},
		{name: "cross org name is locally filtered", before: apiKeyTestObject(apiKeyTestOther, "fixture", apiKeyTestOther), response: apiKeyTestObject(apiKeyTestID, "fixture", apiKeyTestOrg), recovered: apiKeyTestObject(apiKeyTestID, "fixture", apiKeyTestOrg), wantPost: true, wantState: true, explicitOrg: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			posts, deletes, gets := 0, 0, 0
			r := newAPIKeyTestResource(t, func(w http.ResponseWriter, req *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch {
				case req.Method == http.MethodGet && req.URL.Path == "/v1/api_key":
					_, _ = w.Write([]byte(`{"objects":[` + tc.before + `]}`))
				case req.Method == http.MethodGet && req.URL.Path == "/v1/api_key/"+apiKeyTestID:
					gets++
					if tc.recovered == "" {
						w.WriteHeader(http.StatusNotFound)
					} else {
						_, _ = w.Write([]byte(tc.recovered))
					}
				case req.Method == http.MethodPost:
					posts++
					if tc.status != 0 {
						w.WriteHeader(tc.status)
					}
					_, _ = w.Write([]byte(tc.response))
				case req.Method == http.MethodDelete:
					deletes++
					w.WriteHeader(http.StatusNoContent)
				default:
					t.Errorf("unexpected request: %s %s", req.Method, req.URL)
					w.WriteHeader(http.StatusNotFound)
				}
			})
			plan := apiKeyTestPlan()
			if tc.explicitOrg {
				plan.OrgID = types.StringValue(apiKeyTestOrg)
			}
			resp := createAPIKeyTest(t, r, plan)
			if !resp.Diagnostics.HasError() {
				t.Fatal("unsafe or incomplete create must fail")
			}
			if strings.Contains(fmt.Sprint(resp.Diagnostics), apiKeyTestSecret) {
				t.Fatal("diagnostics leaked raw credential")
			}
			if (posts == 1) != tc.wantPost || posts > 1 || deletes != 0 {
				t.Fatalf("unexpected writes: posts=%d deletes=%d", posts, deletes)
			}
			if resp.State.Raw.IsNull() == tc.wantState {
				t.Fatalf("unexpected recovery state; wanted state=%t", tc.wantState)
			}
			if tc.wantState {
				var model apiKeyModel
				if diags := resp.State.Get(context.Background(), &model); diags.HasError() {
					t.Fatal(diags)
				}
				if model.ID.ValueString() != apiKeyTestID || !model.Key.IsNull() || gets != 1 {
					t.Fatal("recovery must retain only verified exact response ID with null secret")
				}
			}
		})
	}
}

func TestAPIKeyMissingReadAndDelete(t *testing.T) {
	for _, status := range []int{http.StatusOK, http.StatusNotFound, http.StatusInternalServerError} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			r := newAPIKeyTestResource(t, func(w http.ResponseWriter, req *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(status)
				if status == http.StatusOK {
					_, _ = w.Write([]byte(apiKeyTestObject(apiKeyTestID, "fixture", apiKeyTestOrg)))
				} else {
					_, _ = fmt.Fprintf(w, `{"error":%q}`, apiKeyTestSecret)
				}
			})
			state := apiKeyTestState(t, r, apiKeyModel{ID: types.StringValue(apiKeyTestID), Key: types.StringValue(apiKeyTestSecret)})
			read := resource.ReadResponse{State: state}
			r.Read(context.Background(), resource.ReadRequest{State: state}, &read)
			if read.State.Raw.IsNull() != (status == http.StatusNotFound) {
				t.Fatal("only missing keys should be removed from state")
			}
			if read.Diagnostics.HasError() != (status == http.StatusInternalServerError) {
				t.Fatal(read.Diagnostics)
			}
			deleted := resource.DeleteResponse{State: state}
			r.Delete(context.Background(), resource.DeleteRequest{State: state}, &deleted)
			if deleted.Diagnostics.HasError() != (status == http.StatusInternalServerError) {
				t.Fatal(deleted.Diagnostics)
			}
			if strings.Contains(fmt.Sprint(read.Diagnostics, deleted.Diagnostics), apiKeyTestSecret) {
				t.Fatal("read/delete errors leaked secret")
			}
		})
	}
}

func TestAPIKeyDataSourceMetadataOnly(t *testing.T) {
	for _, missing := range []bool{false, true} {
		t.Run(fmt.Sprint(missing), func(t *testing.T) {
			r := newAPIKeyTestResource(t, func(w http.ResponseWriter, req *http.Request) {
				if req.Method != http.MethodGet || req.URL.Path != "/v1/api_key/"+apiKeyTestID {
					t.Errorf("unexpected lookup: %s %s", req.Method, req.URL)
				}
				w.Header().Set("Content-Type", "application/json")
				if missing {
					w.WriteHeader(http.StatusNotFound)
					return
				}
				_, _ = w.Write([]byte(apiKeyTestObject(apiKeyTestID, "fixture", apiKeyTestOrg)))
			})
			d := &apiKeyDataSource{client: r.client}
			ctx := context.Background()
			var s datasource.SchemaResponse
			d.Schema(ctx, datasource.SchemaRequest{}, &s)
			if _, ok := s.Schema.Attributes["key"]; ok {
				t.Fatal("lookup exposes secret")
			}
			if _, ok := s.Schema.Attributes["expires_in_seconds"]; ok {
				t.Fatal("lookup synthesizes requested lifetime")
			}
			state := tfsdk.State{Schema: s.Schema}
			if diags := state.Set(ctx, apiKeyDataSourceModel{ID: types.StringValue(apiKeyTestID)}); diags.HasError() {
				t.Fatal(diags)
			}
			resp := datasource.ReadResponse{State: state}
			d.Read(ctx, datasource.ReadRequest{Config: tfsdk.Config{Schema: s.Schema, Raw: state.Raw}}, &resp)
			if resp.Diagnostics.HasError() != missing {
				t.Fatal(resp.Diagnostics)
			}
			if !missing {
				var model apiKeyDataSourceModel
				if diags := resp.State.Get(ctx, &model); diags.HasError() {
					t.Fatal(diags)
				}
				if model.Name.ValueString() != "fixture" || model.UserEmail.ValueString() != "user@example.com" || model.OrgID.ValueString() != apiKeyTestOrg {
					t.Fatal("lookup lost metadata")
				}
			}
		})
	}
}

func TestAPIKeyCanceledCreateGivesCleanupGuidance(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r := newAPIKeyTestResource(t, func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch req.Method {
		case http.MethodGet:
			_, _ = w.Write([]byte(`{"objects":[]}`))
		case http.MethodPost:
			// The server may have minted the key when the caller cancels.
			cancel()
		default:
			t.Errorf("canceled creation must not trigger deletion: %s", req.Method)
		}
	})
	state := apiKeyTestState(t, r, apiKeyTestPlan())
	resp := resource.CreateResponse{State: tfsdk.State{Schema: state.Schema, Raw: tftypes.NewValue(state.Schema.Type().TerraformType(ctx), nil)}}
	r.Create(ctx, resource.CreateRequest{Plan: tfsdk.Plan(state)}, &resp)
	if !resp.Diagnostics.HasError() || !resp.State.Raw.IsNull() {
		t.Fatal("canceled create must report uncertainty without adopting an unverified key")
	}
	detail := fmt.Sprint(resp.Diagnostics)
	if !strings.Contains(detail, "Outcome Uncertain") || !strings.Contains(detail, "revoke only a key created by this attempt") {
		t.Fatal("cancellation lost manual cleanup guidance")
	}
}
