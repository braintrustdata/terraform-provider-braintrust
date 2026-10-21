package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestListAllPagesAndPreservesCallerParams(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Query().Get("limit") != "100" || r.URL.Query().Get("object_type") != "project" {
			t.Errorf("unexpected query: %s", r.URL.RawQuery)
		}
		var rows []map[string]string
		if r.URL.Query().Get("starting_after") == "" {
			for i := 0; i < 100; i++ {
				rows = append(rows, map[string]string{"id": fmt.Sprintf("id-%03d", i)})
			}
		} else {
			if r.URL.Query().Get("starting_after") != "id-099" {
				t.Errorf("unexpected cursor: %s", r.URL.Query().Get("starting_after"))
			}
			rows = []map[string]string{{"id": "id-100"}}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"objects": rows})
	}))
	defer server.Close()
	c, err := NewClient(server.URL, "fixture", "")
	if err != nil {
		t.Fatal(err)
	}
	params := url.Values{"object_type": {"project"}}
	objects, err := c.ListAll(context.Background(), "/v1/acl/list_org", params)
	if err != nil {
		t.Fatal(err)
	}
	if len(objects) != 101 || requests != 2 {
		t.Fatalf("got %d objects in %d requests", len(objects), requests)
	}
	if params.Get("limit") != "" || params.Get("starting_after") != "" {
		t.Fatalf("caller parameters mutated: %v", params)
	}
}

func TestListAllRejectsStalledCursor(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rows := make([]map[string]string, 100)
		for i := range rows {
			rows[i] = map[string]string{"id": "same-id"}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"objects": rows})
	}))
	defer server.Close()
	c, err := NewClient(server.URL, "fixture", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.ListAll(context.Background(), "/v1/acl/list_org", nil); err == nil {
		t.Fatal("expected stalled pagination to fail")
	}
}

func TestListEnvVarsDoesNotSendPaginationParams(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Has("limit") || r.URL.Query().Has("starting_after") {
			t.Errorf("env-var endpoint does not support pagination: %s", r.URL.RawQuery)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"objects": []map[string]string{{
			"id": "env-1", "name": "TEST", "object_type": "project", "object_id": "project-1",
		}}})
	}))
	defer server.Close()
	c, err := NewClient(server.URL, "fixture", "")
	if err != nil {
		t.Fatal(err)
	}
	variables, err := c.ListEnvVars(context.Background(), "project", "project-1")
	if err != nil || len(variables) != 1 {
		t.Fatalf("got %d env vars, error %v", len(variables), err)
	}
}
