package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
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
