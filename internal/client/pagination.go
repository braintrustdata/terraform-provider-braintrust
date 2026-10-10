package client

import (
	"context"
	"fmt"
	"net/url"
)

const listPageSize = 100

// listAllByID follows the standard Braintrust list cursor. Callers still
// apply any filters that the endpoint cannot express (such as org_id).
func listAllByID[T any](ctx context.Context, c *Client, basePath string, filters url.Values, idOf func(T) string) ([]T, error) {
	params := url.Values{}
	for key, values := range filters {
		params[key] = append([]string(nil), values...)
	}
	params.Set("limit", "100")
	var all []T
	previousCursor := ""
	for {
		var page struct {
			Objects []T `json:"objects"`
		}
		if err := c.doRequest(ctx, "GET", basePath+"?"+params.Encode(), nil, &page); err != nil {
			return nil, err
		}
		all = append(all, page.Objects...)
		if len(page.Objects) < listPageSize {
			return all, nil
		}
		cursor := idOf(page.Objects[len(page.Objects)-1])
		if cursor == "" || cursor == previousCursor {
			return nil, fmt.Errorf("%s pagination cursor did not advance", basePath)
		}
		previousCursor = cursor
		params.Set("starting_after", cursor)
	}
}
