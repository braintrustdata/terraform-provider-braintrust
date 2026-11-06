package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const (
	fixtureEnvVarID  = "00000000-0000-0000-0000-000000000001"
	fixtureProjectID = "00000000-0000-0000-0000-000000000002"
	fixtureOrgID     = "00000000-0000-0000-0000-000000000003"
)

func TestListEnvVarsDoesNotSendPaginationParams(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Has("limit") || r.URL.Query().Has("starting_after") {
			t.Errorf("env-var endpoint does not support pagination: %s", r.URL.RawQuery)
		}
		if r.URL.Query().Get("object_type") != "project" || r.URL.Query().Get("object_id") != fixtureProjectID {
			t.Errorf("env-var request missing scope: %s", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"objects": []map[string]string{{
			"id": fixtureEnvVarID, "name": "TEST", "object_type": "project", "object_id": fixtureProjectID,
		}}})
	}))
	defer server.Close()
	c, err := NewClient(server.URL, "fixture", "")
	if err != nil {
		t.Fatal(err)
	}
	variables, err := c.ListEnvVars(context.Background(), "project", fixtureProjectID)
	if err != nil || len(variables) != 1 {
		t.Fatalf("got %d env vars, error %v", len(variables), err)
	}
}

// The pinned spec describes /v1/acl/list_org as returning a bare array; the
// API returns a paginated {"objects": [...]} wrapper.
func TestListOrgACLsDecodesObjectsWrapperAcrossPages(t *testing.T) {
	requests := 0
	acl := func(i int) map[string]string {
		return map[string]string{
			"id": fmt.Sprintf("00000000-0000-0000-0001-%012d", i), "object_type": "project",
			"object_id": fixtureProjectID, "_object_org_id": fixtureOrgID, "permission": "read",
		}
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Path != "/v1/acl/list_org" || r.URL.Query().Get("limit") != "100" {
			t.Errorf("unexpected request: %s", r.URL.String())
		}
		var rows []map[string]string
		if r.URL.Query().Get("starting_after") == "" {
			for i := 0; i < 100; i++ {
				rows = append(rows, acl(i))
			}
		} else {
			rows = []map[string]string{acl(100)}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"objects": rows})
	}))
	defer server.Close()
	c, err := NewClient(server.URL, "fixture", "")
	if err != nil {
		t.Fatal(err)
	}
	acls, err := c.ListOrgACLs(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(acls) != 101 || requests != 2 || acls[100].UnderscoreObjectOrgId.String() != fixtureOrgID {
		t.Fatalf("got %d ACLs in %d requests", len(acls), requests)
	}
}

func TestListDatasetsKeepsRawMetadata(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"objects":[{"id":%q,"project_id":%q,"name":"d","url_slug":"d","metadata":{"big":12345678901234567890}}]}`, fixtureEnvVarID, fixtureProjectID)
	}))
	defer server.Close()
	c, err := NewClient(server.URL, "fixture", "")
	if err != nil {
		t.Fatal(err)
	}
	datasets, err := c.ListDatasets(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(datasets) != 1 || string(datasets[0].Fields["metadata"]) != `{"big":12345678901234567890}` {
		t.Fatalf("metadata was not preserved: %#v", datasets)
	}
}

// IDs passed with --org-id are compared to API IDs as strings, so the verified
// ID must be returned in canonical form.
func TestGetOrganizationReturnsCanonicalID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"objects": []map[string]string{{"id": fixtureOrgID, "name": "org"}}})
	}))
	defer server.Close()
	c, err := NewClient(server.URL, "fixture", "")
	if err != nil {
		t.Fatal(err)
	}
	for _, input := range []string{fixtureOrgID, strings.ToUpper(fixtureOrgID), "{" + fixtureOrgID + "}"} {
		id, err := c.GetOrganization(context.Background(), input)
		if err != nil || id != fixtureOrgID {
			t.Fatalf("GetOrganization(%q) = %q, %v; want %q", input, id, err, fixtureOrgID)
		}
	}
}

func TestListAPIKeysPaginatesMetadata(t *testing.T) {
	requests := 0
	key := func(i int) map[string]string {
		return map[string]string{
			"id":   fmt.Sprintf("00000000-0000-0000-0002-%012d", i),
			"name": "fixture-key", "org_id": fixtureOrgID, "preview_name": "sk-preview",
		}
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.Method != http.MethodGet || r.URL.Path != "/v1/api_key" || r.URL.Query().Get("limit") != "100" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL)
		}
		var rows []map[string]string
		if requests == 1 {
			if r.URL.Query().Has("starting_after") {
				t.Errorf("first page unexpectedly has a cursor: %s", r.URL)
			}
			for i := range 100 {
				rows = append(rows, key(i))
			}
		} else {
			if r.URL.Query().Get("starting_after") != key(99)["id"] {
				t.Errorf("wrong pagination cursor: %s", r.URL)
			}
			rows = []map[string]string{key(100)}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"objects": rows})
	}))
	defer server.Close()
	c, err := NewClient(server.URL, "fixture", "")
	if err != nil {
		t.Fatal(err)
	}
	keys, err := c.ListAPIKeys(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 101 || requests != 2 || keys[100].Id.String() != key(100)["id"] {
		t.Fatalf("got %d API keys in %d requests", len(keys), requests)
	}
}
