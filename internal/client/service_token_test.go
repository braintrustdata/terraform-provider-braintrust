package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestListServiceTokensByNamePaginates(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		requests++
		if req.URL.Path != "/v1/service_token" || req.URL.Query().Get("name") != "fixture" || req.URL.Query().Get("limit") != "100" {
			t.Errorf("unexpected list request: %s", req.URL.String())
		}
		objects := make([]ServiceToken, 0, 100)
		if requests == 1 {
			for i := 0; i < 100; i++ {
				objects = append(objects, ServiceToken{ID: fmt.Sprintf("token-%03d", i), Name: "fixture"})
			}
		} else if req.URL.Query().Get("starting_after") != "token-099" {
			t.Errorf("wrong cursor: %s", req.URL.String())
		} else {
			objects = append(objects, ServiceToken{ID: "token-100", Name: "fixture"})
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"objects": objects})
	}))
	defer server.Close()
	c, err := NewClient(server.URL, "fixture-key", "")
	if err != nil {
		t.Fatal(err)
	}
	tokens, err := c.ListServiceTokensByName(context.Background(), "fixture")
	if err != nil {
		t.Fatal(err)
	}
	if requests != 2 || len(tokens) != 101 || tokens[100].ID != "token-100" {
		t.Fatalf("got %d requests and %d tokens", requests, len(tokens))
	}
}
