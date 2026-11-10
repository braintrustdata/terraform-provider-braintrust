package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync"
	"testing"

	"github.com/braintrustdata/terraform-provider-braintrust/internal/client"
	frameworkresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// A server error after external account deletion must not make Terraform forget
// the ACL. Only an actual not-found response permits forgetting it.
func TestACLReadAndDeleteErrors(t *testing.T) {
	for _, tc := range []struct {
		name      string
		status    int
		body      string
		wantError bool
	}{
		{
			name: "server_error", status: http.StatusInternalServerError,
			body: `{"error":"user does not exist"}`, wantError: true,
		},
		{
			name: "not_found", status: http.StatusNotFound,
			body: `{"error":"ACL not found"}`,
		},
		{
			name: "missing_acl_bad_request", status: http.StatusBadRequest,
			body: `{"error":"Missing read access to acl 00000000-0000-0000-0000-000000000001 or the acl does not exist"}`,
		},
		{
			name: "other_bad_request", status: http.StatusBadRequest,
			body: `{"error":"Invalid ACL request"}`, wantError: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				if req.URL.Path != "/v1/acl/00000000-0000-0000-0000-000000000001" || (req.Method != http.MethodGet && req.Method != http.MethodDelete) {
					t.Errorf("unexpected request: %s %s", req.Method, req.URL)
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()
			c, err := client.NewClient(server.URL, "fixture", "")
			if err != nil {
				t.Fatal(err)
			}
			ctx := context.Background()
			r := &aclResource{client: c}
			var schemaResp frameworkresource.SchemaResponse
			r.Schema(ctx, frameworkresource.SchemaRequest{}, &schemaResp)
			state := tfsdk.State{Schema: schemaResp.Schema}
			if diags := state.Set(ctx, aclModel{
				ID:         types.StringValue("00000000-0000-0000-0000-000000000001"),
				ObjectType: types.StringValue("project"), ObjectID: types.StringValue("00000000-0000-0000-0000-000000000002"),
				UserID: types.StringValue("00000000-0000-0000-0000-000000000003"), Permission: types.StringValue("read"),
			}); diags.HasError() {
				t.Fatal(diags)
			}

			readResp := frameworkresource.ReadResponse{State: state}
			r.Read(ctx, frameworkresource.ReadRequest{State: state}, &readResp)
			deleteResp := frameworkresource.DeleteResponse{State: state}
			r.Delete(ctx, frameworkresource.DeleteRequest{State: state}, &deleteResp)
			if readResp.Diagnostics.HasError() != tc.wantError || deleteResp.Diagnostics.HasError() != tc.wantError {
				t.Fatalf("unexpected diagnostics: read=%v delete=%v", readResp.Diagnostics, deleteResp.Diagnostics)
			}
			if tc.wantError {
				if !readResp.State.Raw.Equal(state.Raw) || !deleteResp.State.Raw.Equal(state.Raw) {
					t.Fatal("API error discarded or changed ACL state")
				}
			} else if !readResp.State.Raw.IsNull() {
				t.Fatal("not-found ACL was not removed during refresh")
			}
		})
	}
}

// Exercise the real Terraform graph during both replacement and final destroy.
// The fixture rejects account removal while its ACL still exists.
func TestACLServiceAccountDeletionOrder(t *testing.T) {
	const (
		projectID = "00000000-0000-0000-0000-000000000001"
		orgID     = "00000000-0000-0000-0000-000000000002"
	)
	var mu sync.Mutex
	var accountID string
	var acl map[string]string
	var events []string
	generation := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch {
		case req.Method == http.MethodPatch && req.URL.Path == "/v1/organization/members":
			var change struct {
				InviteUsers *struct {
					ServiceAccounts []struct {
						Name string `json:"name"`
					} `json:"service_accounts"`
				} `json:"invite_users"`
				RemoveUsers *struct {
					IDs []string `json:"ids"`
				} `json:"remove_users"`
			}
			if err := json.NewDecoder(req.Body).Decode(&change); err != nil {
				t.Error(err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			if change.InviteUsers != nil && len(change.InviteUsers.ServiceAccounts) == 1 {
				if accountID != "" {
					t.Error("account created before old account was removed")
				}
				generation++
				accountID = fmt.Sprintf("00000000-0000-0000-0001-%012d", generation)
				events = append(events, fmt.Sprintf("create account %d", generation))
				_ = json.NewEncoder(w).Encode(map[string]any{"added_users": []map[string]string{{"id": accountID, "email": "fixture@service.invalid"}}})
			} else if change.RemoveUsers != nil && slices.Equal(change.RemoveUsers.IDs, []string{accountID}) && accountID != "" {
				if acl != nil {
					t.Error("account removed before its ACL")
					w.WriteHeader(http.StatusInternalServerError)
					return
				}
				events = append(events, fmt.Sprintf("delete account %d", generation))
				accountID = ""
				_, _ = w.Write([]byte(`{}`))
			} else {
				t.Error("unexpected membership change")
				w.WriteHeader(http.StatusBadRequest)
			}
		case req.Method == http.MethodGet && req.URL.Path == "/v1/acl":
			objects := []map[string]string{}
			if acl != nil {
				objects = append(objects, acl)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"objects": objects})
		case req.Method == http.MethodPost && req.URL.Path == "/v1/acl":
			var input map[string]any
			if err := json.NewDecoder(req.Body).Decode(&input); err != nil {
				t.Error(err)
			}
			if accountID == "" || input["user_id"] != accountID || acl != nil {
				t.Error("ACL must reference the current account and replace the old ACL")
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			acl = map[string]string{
				"id":          fmt.Sprintf("00000000-0000-0000-0002-%012d", generation),
				"object_type": "project", "object_id": projectID, "_object_org_id": orgID,
				"user_id": accountID, "permission": "read",
			}
			events = append(events, fmt.Sprintf("create ACL %d", generation))
			_ = json.NewEncoder(w).Encode(acl)
		case acl != nil && req.URL.Path == "/v1/acl/"+acl["id"] && req.Method == http.MethodGet:
			_ = json.NewEncoder(w).Encode(acl)
		case acl != nil && req.URL.Path == "/v1/acl/"+acl["id"] && req.Method == http.MethodDelete:
			_ = json.NewEncoder(w).Encode(acl)
			acl = nil
			events = append(events, fmt.Sprintf("delete ACL %d", generation))
		default:
			t.Errorf("unexpected request: %s %s", req.Method, req.URL)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	config := func(name string) string {
		return fmt.Sprintf(`
provider "braintrust" {
  api_key = "fixture"
  api_url = %q
}
resource "braintrust_service_account" "test" {
  name = %q
}
resource "braintrust_acl" "test" {
  object_type = "project"
  object_id   = %q
  user_id     = braintrust_service_account.test.id
  permission  = "read"
}
`, server.URL, name, projectID)
	}
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: config("original")},
			{Config: config("replacement")},
		},
	})
	mu.Lock()
	defer mu.Unlock()
	want := []string{
		"create account 1", "create ACL 1", "delete ACL 1", "delete account 1",
		"create account 2", "create ACL 2", "delete ACL 2", "delete account 2",
	}
	if !slices.Equal(events, want) || accountID != "" || acl != nil {
		t.Fatalf("incorrect lifecycle: events=%v account=%q ACL=%v", events, accountID, acl)
	}
}
