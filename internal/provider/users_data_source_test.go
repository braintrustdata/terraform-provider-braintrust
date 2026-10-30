package provider

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestUsersDataSource(t *testing.T) {
	const aliceID, bobID = "00000000-0000-0000-0000-0000000000a1", "00000000-0000-0000-0000-0000000000b1"
	users := map[string]string{
		aliceID: `{"id":"` + aliceID + `","email":"alice@example.com","given_name":"Alice"}`,
		bobID:   `{"id":"` + bobID + `","email":"bob@example.com"}`,
	}
	emails := map[string]string{"alice@example.com": aliceID, "bob@example.com": bobID}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/user" {
			t.Errorf("unexpected request: %s", r.URL.String())
			w.WriteHeader(http.StatusNotFound)
			return
		}
		query := r.URL.Query()
		var objects []string
		for _, id := range []string{aliceID, bobID} {
			if query.Has("ids") && !slices.Contains(query["ids"], id) {
				continue
			}
			if query.Has("email") && !slices.ContainsFunc(query["email"], func(e string) bool { return emails[e] == id }) {
				continue
			}
			objects = append(objects, users[id])
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"objects":[%s]}`, strings.Join(objects, ","))
	}))
	defer server.Close()

	prefix := fmt.Sprintf("provider \"braintrust\" {\n  api_key = \"fixture\"\n  api_url = %q\n}\n", server.URL)
	const name = "data.braintrust_users.test"
	tests := []struct {
		name   string
		config string
		check  resource.TestCheckFunc
	}{
		{"all users", `data "braintrust_users" "test" {}`, resource.ComposeAggregateTestCheckFunc(
			resource.TestCheckResourceAttr(name, "users.#", "2"),
			resource.TestCheckResourceAttr(name, "users.0.email", "alice@example.com"),
			resource.TestCheckResourceAttr(name, "users.0.given_name", "Alice"),
			resource.TestCheckResourceAttr(name, "users.1.id", bobID),
		)},
		{"emails skip missing users", `data "braintrust_users" "test" { emails = ["bob@example.com", "nobody@example.com"] }`, resource.ComposeAggregateTestCheckFunc(
			resource.TestCheckResourceAttr(name, "users.#", "1"),
			resource.TestCheckResourceAttr(name, "users.0.id", bobID),
		)},
		{"ids", `data "braintrust_users" "test" { ids = ["` + aliceID + `"] }`, resource.ComposeAggregateTestCheckFunc(
			resource.TestCheckResourceAttr(name, "users.#", "1"),
			resource.TestCheckResourceAttr(name, "users.0.email", "alice@example.com"),
		)},
		{"empty emails", `data "braintrust_users" "test" { emails = [] }`, resource.TestCheckResourceAttr(name, "users.#", "0")},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Steps:                    []resource.TestStep{{Config: prefix + test.config, Check: test.check}},
			})
		})
	}
}
