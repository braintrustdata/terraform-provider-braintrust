package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestViewRequestsRequireScopeAndRejectExistingCreate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			w.Header().Set("x-bt-found-existing", "true")
			_, _ = w.Write([]byte(`{"id":"existing"}`))
		case http.MethodGet:
			if r.URL.Query().Get("object_type") != "project" || r.URL.Query().Get("object_id") != "parent" {
				t.Errorf("GET missing view scope: %s", r.URL.String())
			}
			if r.URL.Path == "/v1/view" {
				if r.URL.Query().Get("view_name") != "same" {
					t.Errorf("GET missing view_name filter: %s", r.URL.String())
				}
				_, _ = w.Write([]byte(`{"objects":[{"id":"view","name":"same"}]}`))
			} else {
				_, _ = w.Write([]byte(`{"id":"view"}`))
			}
		case http.MethodDelete:
			var body map[string]string
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body["object_type"] != "project" || body["object_id"] != "parent" {
				t.Errorf("DELETE missing view scope: %v, %v", body, err)
			}
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	defer server.Close()
	c, err := NewClient(server.URL, "fixture", "")
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.CreateView(context.Background(), CreateViewRequest{ObjectType: "project", ObjectID: "parent", Name: "same"})
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("expected existing-view error, got %v", err)
	}
	if _, err := c.GetView(context.Background(), "view", "project", "parent"); err != nil {
		t.Fatal(err)
	}
	if views, err := c.ListViews(context.Background(), "project", "parent", "same"); err != nil || len(views) != 1 {
		t.Fatalf("list views: %v, %v", views, err)
	}
	if err := c.DeleteView(context.Background(), "view", "project", "parent"); err != nil {
		t.Fatal(err)
	}
}
