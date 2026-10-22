package client

import (
	"fmt"
	"testing"
)

func TestListAllFollowsCursorAcrossPages(t *testing.T) {
	var cursors []string
	items, err := ListAll("/v1/thing", func(limit int, startingAfter *string) ([]string, error) {
		if limit != listPageSize {
			t.Errorf("limit = %d, want %d", limit, listPageSize)
		}
		cursor := ""
		if startingAfter != nil {
			cursor = *startingAfter
		}
		cursors = append(cursors, cursor)
		if cursor == "" {
			page := make([]string, listPageSize)
			for i := range page {
				page[i] = fmt.Sprintf("item-%03d", i)
			}
			return page, nil
		}
		return []string{"last"}, nil
	}, func(item string) string { return item })
	if err != nil || len(items) != listPageSize+1 || items[len(items)-1] != "last" {
		t.Fatalf("want %d items ending in last; got %d err=%v", listPageSize+1, len(items), err)
	}
	if len(cursors) != 2 || cursors[1] != "item-099" {
		t.Fatalf("unexpected cursors: %q", cursors)
	}
}

func TestListAllRejectsStuckCursor(t *testing.T) {
	_, err := ListAll("/v1/thing", func(limit int, _ *string) ([]string, error) {
		page := make([]string, limit)
		for i := range page {
			page[i] = "same"
		}
		return page, nil
	}, func(item string) string { return item })
	if err == nil {
		t.Fatal("want an error when the cursor does not advance")
	}
}
