package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

const viewBasePath = "/v1/view"

// View is a saved view or monitor dashboard. Charts are part of ViewData.
type View struct {
	ID          string          `json:"id"`
	ObjectType  string          `json:"object_type"`
	ObjectID    string          `json:"object_id"`
	ViewType    string          `json:"view_type"`
	Name        string          `json:"name"`
	Description *string         `json:"description"`
	Starred     *bool           `json:"starred"`
	ViewData    json.RawMessage `json:"view_data"`
	Options     json.RawMessage `json:"options"`
	Created     *string         `json:"created"`
	UpdatedAt   *string         `json:"updated_at"`
	UserID      *string         `json:"user_id"`
}

type CreateViewRequest struct {
	ObjectType  string          `json:"object_type"`
	ObjectID    string          `json:"object_id"`
	ViewType    string          `json:"view_type"`
	Name        string          `json:"name"`
	Description *string         `json:"description,omitempty"`
	ViewData    json.RawMessage `json:"view_data,omitempty"`
	Options     json.RawMessage `json:"options,omitempty"`
}

type UpdateViewRequest struct {
	ObjectType  string           `json:"object_type"`
	ObjectID    string           `json:"object_id"`
	Name        *string          `json:"name,omitempty"`
	Description *string          `json:"description,omitempty"`
	Starred     *bool            `json:"starred,omitempty"`
	ViewData    *json.RawMessage `json:"view_data,omitempty"`
	Options     *json.RawMessage `json:"options,omitempty"`
}

func (c *Client) CreateView(ctx context.Context, req CreateViewRequest) (*View, error) {
	var view View
	var headers http.Header
	if err := c.doRequestWithHeaders(ctx, "POST", viewBasePath, req, &view, &headers); err != nil {
		return nil, err
	}
	if headers.Get("x-bt-found-existing") != "" {
		return nil, fmt.Errorf("view %q already exists in %s %s; import the existing view instead", req.Name, req.ObjectType, req.ObjectID)
	}
	return &view, nil
}

func (c *Client) GetView(ctx context.Context, id, objectType, objectID string) (*View, error) {
	if id == "" || objectType == "" || objectID == "" {
		return nil, fmt.Errorf("view id, object type, and object id are required")
	}
	var view View
	params := url.Values{"object_type": {objectType}, "object_id": {objectID}}
	if err := c.doRequest(ctx, "GET", viewBasePath+"/"+url.PathEscape(id)+"?"+params.Encode(), nil, &view); err != nil {
		return nil, err
	}
	return &view, nil
}

func (c *Client) ListViews(ctx context.Context, objectType, objectID, name string) ([]View, error) {
	params := url.Values{"limit": {"100"}}
	if objectType != "" {
		params.Set("object_type", objectType)
	}
	if objectID != "" {
		params.Set("object_id", objectID)
	}
	if name != "" {
		params.Set("view_name", name)
	}
	var all []View
	var previousCursor string
	for {
		var result struct {
			Objects []View `json:"objects"`
		}
		if err := c.doRequest(ctx, "GET", viewBasePath+"?"+params.Encode(), nil, &result); err != nil {
			return nil, err
		}
		all = append(all, result.Objects...)
		if len(result.Objects) < 100 {
			return all, nil
		}
		cursor := result.Objects[len(result.Objects)-1].ID
		if cursor == "" || cursor == previousCursor {
			return nil, fmt.Errorf("view pagination cursor did not advance")
		}
		previousCursor = cursor
		params.Set("starting_after", cursor)
	}
}

func (c *Client) UpdateView(ctx context.Context, id string, req UpdateViewRequest) (*View, error) {
	if id == "" {
		return nil, fmt.Errorf("view id is required")
	}
	var view View
	if err := c.doRequest(ctx, "PATCH", viewBasePath+"/"+url.PathEscape(id), req, &view); err != nil {
		return nil, err
	}
	return &view, nil
}

func (c *Client) DeleteView(ctx context.Context, id, objectType, objectID string) error {
	if id == "" || objectType == "" || objectID == "" {
		return fmt.Errorf("view id, object type, and object id are required")
	}
	return c.doRequest(ctx, "DELETE", viewBasePath+"/"+url.PathEscape(id), map[string]string{"object_type": objectType, "object_id": objectID}, nil)
}
