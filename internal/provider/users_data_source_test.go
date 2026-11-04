package provider

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
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
		err    string
	}{
		{name: "return all", config: `data "braintrust_users" "test" { return_all = true }`, check: resource.ComposeAggregateTestCheckFunc(
			resource.TestCheckResourceAttr(name, "users.#", "2"),
			resource.TestCheckResourceAttr(name, "users.0.email", "alice@example.com"),
			resource.TestCheckResourceAttr(name, "users.0.given_name", "Alice"),
			resource.TestCheckResourceAttr(name, "users.1.id", bobID),
		)},
		{name: "emails", config: `data "braintrust_users" "test" { emails = ["bob@example.com", "alice@example.com"] }`, check: resource.ComposeAggregateTestCheckFunc(
			resource.TestCheckResourceAttr(name, "users.#", "2"),
			resource.TestCheckResourceAttr(name, "users.1.id", bobID),
			resource.TestCheckResourceAttr(name, "missing_emails.#", "0"),
		)},
		{name: "missing email fails", config: `data "braintrust_users" "test" { emails = ["bob@example.com", "nobody@example.com"] }`, err: `No user found for: nobody@example.com`},
		{name: "ignore missing emails", config: `data "braintrust_users" "test" {
  emails         = ["bob@example.com", "nobody@example.com"]
  ignore_missing = true
}`, check: resource.ComposeAggregateTestCheckFunc(
			resource.TestCheckResourceAttr(name, "users.#", "1"),
			resource.TestCheckResourceAttr(name, "users.0.id", bobID),
			resource.TestCheckResourceAttr(name, "missing_emails.#", "1"),
			resource.TestCheckTypeSetElemAttr(name, "missing_emails.*", "nobody@example.com"),
		)},
		{name: "ids", config: `data "braintrust_users" "test" { ids = ["` + aliceID + `"] }`, check: resource.ComposeAggregateTestCheckFunc(
			resource.TestCheckResourceAttr(name, "users.#", "1"),
			resource.TestCheckResourceAttr(name, "users.0.email", "alice@example.com"),
		)},
		{name: "missing id fails", config: `data "braintrust_users" "test" { ids = ["00000000-0000-0000-0000-0000000000c1"] }`, err: `No user found for: 00000000-0000-0000-0000-0000000000c1`},
		{name: "empty emails", config: `data "braintrust_users" "test" { emails = [] }`, check: resource.TestCheckResourceAttr(name, "users.#", "0")},
		{name: "no filter fails", config: `data "braintrust_users" "test" {}`, err: `Missing Attribute Configuration`},
		{name: "two filters fail", config: `data "braintrust_users" "test" {
  emails = ["bob@example.com"]
  ids    = ["` + bobID + `"]
}`, err: `Invalid Attribute Combination`},
		{name: "return_all false fails", config: `data "braintrust_users" "test" { return_all = false }`, err: `return_all must be true when set`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			step := resource.TestStep{Config: prefix + test.config, Check: test.check}
			if test.err != "" {
				step.ExpectError = regexp.MustCompile(test.err)
			}
			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Steps:                    []resource.TestStep{step},
			})
		})
	}
}

func TestUsersDataSourceBatching(t *testing.T) {
	const name = "data.braintrust_users.test"
	// Exercise the real 100-filter boundary without changing the production limit.
	emails := make([]string, 101)
	quotedEmails := make([]string, len(emails))
	ids := make([]string, len(emails))
	quotedIDs := make([]string, len(emails))
	emailsByID := make(map[string]string, len(emails))
	userIDs := make(map[string]string, len(emails))
	for i := range emails {
		emails[i] = fmt.Sprintf("user%03d@example.com", i)
		quotedEmails[i] = fmt.Sprintf("%q", emails[i])
		userIDs[emails[i]] = fmt.Sprintf("00000000-0000-0000-0000-%012d", i+1)
		ids[i] = userIDs[emails[i]]
		quotedIDs[i] = fmt.Sprintf("%q", ids[i])
		emailsByID[ids[i]] = emails[i]
	}

	for _, test := range []struct {
		name          string
		byID          bool
		missing       bool
		ignoreMissing bool
	}{
		{name: "all users"},
		{name: "all users by ID", byID: true},
		{name: "missing user in second batch", missing: true},
		{name: "ignore missing user in second batch", missing: true, ignoreMissing: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			attribute, queryKey := "emails", "email"
			quotedFilters, expectedFilters := quotedEmails, emails
			if test.byID {
				attribute, queryKey = "ids", "ids"
				quotedFilters, expectedFilters = quotedIDs, ids
			}
			var mu sync.Mutex
			var batches [][]string
			var missingEmail string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				defer mu.Unlock()
				query := r.URL.Query()
				filters := query[queryKey]
				if r.URL.Path != "/v1/user" || len(filters) == 0 || len(filters) > 100 {
					t.Errorf("unexpected request: %s", r.URL.String())
					http.Error(w, "expected 1 to 100 filters", http.StatusBadRequest)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				// A full 100-user page causes a follow-up pagination request,
				// not another filter batch.
				if query.Has("starting_after") {
					_, _ = fmt.Fprint(w, `{"objects":[]}`)
					return
				}
				batches = append(batches, filters)
				var objects []string
				for _, filter := range filters {
					email := filter
					if test.byID {
						email = emailsByID[filter]
					}
					id, ok := userIDs[email]
					if !ok {
						t.Errorf("unexpected %s filter: %q", queryKey, filter)
						http.Error(w, "unknown filter", http.StatusBadRequest)
						return
					}
					// Omit the singleton batch's user rather than depending
					// on Terraform's set iteration order.
					if test.missing && len(filters) == 1 {
						missingEmail = email
						continue
					}
					objects = append(objects, fmt.Sprintf(`{"id":%q,"email":%q}`, id, email))
				}
				_, _ = fmt.Fprintf(w, `{"objects":[%s]}`, strings.Join(objects, ","))
			}))
			defer server.Close()

			step := resource.TestStep{
				Config: fmt.Sprintf(`
provider "braintrust" {
  api_key = "fixture"
  api_url = %q
}
data "braintrust_users" "test" {
  %s             = [%s]
  ignore_missing = %t
}
`, server.URL, attribute, strings.Join(quotedFilters, ","), test.ignoreMissing),
				Check: func(state *terraform.State) error {
					mu.Lock()
					defer mu.Unlock()
					checks := []resource.TestCheckFunc{
						resource.TestCheckResourceAttr(name, "missing_ids.#", "0"),
					}
					index := 0
					for _, email := range emails {
						if email == missingEmail {
							continue
						}
						checks = append(checks,
							resource.TestCheckResourceAttr(name, fmt.Sprintf("users.%d.email", index), email),
							resource.TestCheckResourceAttr(name, fmt.Sprintf("users.%d.id", index), userIDs[email]),
						)
						index++
					}
					checks = append(checks, resource.TestCheckResourceAttr(name, "users.#", fmt.Sprint(index)))
					if test.missing {
						if missingEmail == "" {
							return fmt.Errorf("the second batch was not requested")
						}
						checks = append(checks,
							resource.TestCheckResourceAttr(name, "missing_emails.#", "1"),
							resource.TestCheckTypeSetElemAttr(name, "missing_emails.*", missingEmail),
						)
					} else {
						checks = append(checks, resource.TestCheckResourceAttr(name, "missing_emails.#", "0"))
					}
					return resource.ComposeAggregateTestCheckFunc(checks...)(state)
				},
			}
			if test.missing && !test.ignoreMissing {
				step.ExpectError = regexp.MustCompile(`No user found for: user[0-9]{3}@example\.com`)
			}
			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Steps:                    []resource.TestStep{step},
			})

			mu.Lock()
			defer mu.Unlock()
			// Terraform can read more than once during plan/apply. Each read
			// must request every filter exactly once across two batches.
			if len(batches) == 0 || len(batches)%2 != 0 {
				t.Fatalf("expected pairs of filter batches, got %d batches", len(batches))
			}
			for i := 0; i < len(batches); i += 2 {
				if len(batches[i]) != 100 || len(batches[i+1]) != 1 {
					t.Fatalf("batch sizes = %d, %d; want 100, 1", len(batches[i]), len(batches[i+1]))
				}
				requested := append(slices.Clone(batches[i]), batches[i+1]...)
				slices.Sort(requested)
				if !slices.Equal(requested, expectedFilters) {
					t.Fatalf("batched filters = %v; want %v", requested, expectedFilters)
				}
			}
		})
	}
}

func TestUsersDataSourcePagination(t *testing.T) {
	const name = "data.braintrust_users.test"
	objects := make([]string, 101)
	ids := make([]string, len(objects))
	checks := []resource.TestCheckFunc{
		resource.TestCheckResourceAttr(name, "users.#", "101"),
		resource.TestCheckResourceAttr(name, "missing_emails.#", "0"),
		resource.TestCheckResourceAttr(name, "missing_ids.#", "0"),
	}
	for i := range objects {
		ids[i] = fmt.Sprintf("00000000-0000-0000-0000-%012d", i+1)
		email := fmt.Sprintf("user%03d@example.com", i)
		objects[i] = fmt.Sprintf(`{"id":%q,"email":%q}`, ids[i], email)
		checks = append(checks,
			resource.TestCheckResourceAttr(name, fmt.Sprintf("users.%d.id", i), ids[i]),
			resource.TestCheckResourceAttr(name, fmt.Sprintf("users.%d.email", i), email),
		)
	}
	// Return reverse email order so the second page's user must be included
	// and sorted ahead of every user from the first page.
	slices.Reverse(objects)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/user" {
			t.Errorf("unexpected request: %s", r.URL.String())
			http.Error(w, "unexpected path", http.StatusNotFound)
			return
		}
		var page []string
		switch cursor := r.URL.Query().Get("starting_after"); cursor {
		case "":
			page = objects[:100]
		case ids[1]:
			// The last object in the first page is user001.
			page = objects[100:]
		default:
			t.Errorf("unexpected pagination cursor: %q; want %q", cursor, ids[1])
			http.Error(w, "invalid cursor", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"objects":[%s]}`, strings.Join(page, ","))
	}))
	defer server.Close()

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{{
			Config: fmt.Sprintf(`
provider "braintrust" {
  api_key = "fixture"
  api_url = %q
}
data "braintrust_users" "test" {
  return_all = true
}
`, server.URL),
			Check: resource.ComposeAggregateTestCheckFunc(checks...),
		}},
	})
}
