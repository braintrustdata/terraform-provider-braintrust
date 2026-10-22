package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/braintrustdata/terraform-provider-braintrust/internal/client"
)

func TestListServiceTokensByNamePaginates(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		requests++
		if req.URL.Path != "/v1/service_token" || req.URL.Query().Get("name") != "fixture" || req.URL.Query().Get("limit") != "100" {
			t.Errorf("unexpected list request: %s", req.URL.String())
		}
		objects := make([]map[string]string, 0, 100)
		token := func(i int) map[string]string {
			return map[string]string{"id": fmt.Sprintf("00000000-0000-0000-0001-%012d", i), "name": "fixture", "preview_name": "bt-st-…"}
		}
		if requests == 1 {
			for i := 0; i < 100; i++ {
				objects = append(objects, token(i))
			}
		} else if req.URL.Query().Get("starting_after") != "00000000-0000-0000-0001-000000000099" {
			t.Errorf("wrong cursor: %s", req.URL.String())
		} else {
			objects = append(objects, token(100))
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"objects": objects})
	}))
	defer server.Close()
	c, err := client.NewClient(server.URL, "fixture-key", "")
	if err != nil {
		t.Fatal(err)
	}
	tokens, err := listServiceTokensByName(context.Background(), c, "fixture")
	if err != nil {
		t.Fatal(err)
	}
	if requests != 2 || len(tokens) != 101 || tokens[100].Id.String() != "00000000-0000-0000-0001-000000000100" {
		t.Fatalf("got %d requests and %d tokens", requests, len(tokens))
	}
}
