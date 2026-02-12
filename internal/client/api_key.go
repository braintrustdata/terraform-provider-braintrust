package client

import (
	"context"
	"fmt"
	"net/url"
)

const apiKeyBasePath = "/v1/api_key"

// APIKey represents a Braintrust API key (GET response — no raw key).
type APIKey struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	PreviewName string  `json:"preview_name"`
	Created     *string `json:"created,omitempty"`
	UserID      *string `json:"user_id,omitempty"`
	OrgID       *string `json:"org_id,omitempty"`
}

// CreateAPIKeyOutput is the POST response, which includes the raw key.
// The raw key is only exposed at creation time.
type CreateAPIKeyOutput struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	PreviewName string  `json:"preview_name"`
	Key         string  `json:"key"`
	Created     *string `json:"created,omitempty"`
	UserID      *string `json:"user_id,omitempty"`
	OrgID       *string `json:"org_id,omitempty"`
}

// CreateAPIKeyRequest is the request body for creating an API key.
type CreateAPIKeyRequest struct {
	Name string `json:"name"`
}

// CreateAPIKey creates a new API key. Returns the raw key (only exposed once).
func (c *Client) CreateAPIKey(ctx context.Context, req CreateAPIKeyRequest) (*CreateAPIKeyOutput, error) {
	var result CreateAPIKeyOutput
	if err := c.doRequest(ctx, "POST", apiKeyBasePath, req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// GetAPIKey retrieves an API key by ID. The raw key is NOT included.
func (c *Client) GetAPIKey(ctx context.Context, id string) (*APIKey, error) {
	if id == "" {
		return nil, fmt.Errorf("api_key id is required")
	}
	var apiKey APIKey
	if err := c.doRequest(ctx, "GET", apiKeyBasePath+"/"+url.PathEscape(id), nil, &apiKey); err != nil {
		return nil, err
	}
	return &apiKey, nil
}

// DeleteAPIKey deletes an API key by ID.
func (c *Client) DeleteAPIKey(ctx context.Context, id string) error {
	if id == "" {
		return fmt.Errorf("api_key id is required")
	}
	return c.doRequest(ctx, "DELETE", apiKeyBasePath+"/"+url.PathEscape(id), nil, nil)
}
