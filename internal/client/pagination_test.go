package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNameLookupsReadLaterPages(t *testing.T) {
	for _, test := range []struct {
		name, path, filter string
		list               func(context.Context, *Client) (int, string, error)
	}{
		{"dataset", "/v1/dataset", "dataset_name", func(ctx context.Context, c *Client) (int, string, error) {
			items, err := c.ListDatasets(ctx, "Shared", "project-a")
			if err != nil {
				return 0, "", err
			}
			return len(items), items[len(items)-1].ID, nil
		}},
		{"prompt", "/v1/prompt", "prompt_name", func(ctx context.Context, c *Client) (int, string, error) {
			items, err := c.ListPrompts(ctx, "Shared", "project-a")
			if err != nil {
				return 0, "", err
			}
			return len(items), items[len(items)-1].ID, nil
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				q := r.URL.Query()
				if r.URL.Path != test.path || q.Get(test.filter) != "Shared" || q.Get("project_id") != "project-a" || q.Get("limit") != "100" {
					t.Errorf("unexpected list request: %s", r.URL.String())
				}
				w.Header().Set("Content-Type", "application/json")
				if q.Get("starting_after") == "" {
					objects := make([]map[string]string, 100)
					for i := range objects {
						objects[i] = map[string]string{"id": fmt.Sprintf("item-%03d", i), "name": "Shared", "project_id": "project-a"}
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"objects": objects})
					return
				}
				if q.Get("starting_after") != "item-099" {
					t.Errorf("wrong cursor: %s", r.URL.String())
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"objects": []map[string]string{{"id": "last", "name": "Shared", "project_id": "project-a"}}})
			}))
			defer server.Close()
			c, err := NewClient(server.URL, "fixture", "")
			if err != nil {
				t.Fatal(err)
			}
			count, last, err := test.list(context.Background(), c)
			if err != nil || count != 101 || last != "last" || requests != 2 {
				t.Fatalf("want 101 objects across two pages; count=%d last=%q requests=%d error=%v", count, last, requests, err)
			}
		})
	}
}
