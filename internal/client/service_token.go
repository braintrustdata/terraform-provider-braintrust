package client

import (
	"context"
	"fmt"
	"net/url"
)

const serviceTokenBasePath = "/v1/service_token"

// ServiceToken represents a Braintrust service token (GET response — no raw key).
type ServiceToken struct {
	ID                  string  `json:"id"`
	Name                string  `json:"name"`
	PreviewName         string  `json:"preview_name"`
	Created             *string `json:"created,omitempty"`
	ServiceAccountID    *string `json:"service_account_id,omitempty"`
	ServiceAccountEmail *string `json:"service_account_email,omitempty"`
	ServiceAccountName  *string `json:"service_account_name,omitempty"`
	OrgID               *string `json:"org_id,omitempty"`
}

// CreateServiceTokenOutput is the POST response, which includes the raw key.
type CreateServiceTokenOutput struct {
	ID                  string  `json:"id"`
	Name                string  `json:"name"`
	PreviewName         string  `json:"preview_name"`
	Key                 string  `json:"key"`
	Created             *string `json:"created,omitempty"`
	ServiceAccountID    *string `json:"service_account_id,omitempty"`
	ServiceAccountEmail *string `json:"service_account_email,omitempty"`
	ServiceAccountName  *string `json:"service_account_name,omitempty"`
	OrgID               *string `json:"org_id,omitempty"`
}

// CreateServiceTokenRequest is the request body for creating a service token.
type CreateServiceTokenRequest struct {
	Name             string `json:"name"`
	ServiceAccountID string `json:"service_account_id"`
}

// CreateServiceToken creates a new service token. Returns the raw key (only exposed once).
func (c *Client) CreateServiceToken(ctx context.Context, req CreateServiceTokenRequest) (*CreateServiceTokenOutput, error) {
	var result CreateServiceTokenOutput
	if err := c.doRequest(ctx, "POST", serviceTokenBasePath, req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// GetServiceToken retrieves a service token by ID. The raw key is NOT included.
func (c *Client) GetServiceToken(ctx context.Context, id string) (*ServiceToken, error) {
	if id == "" {
		return nil, fmt.Errorf("service_token id is required")
	}
	var token ServiceToken
	if err := c.doRequest(ctx, "GET", serviceTokenBasePath+"/"+url.PathEscape(id), nil, &token); err != nil {
		return nil, err
	}
	return &token, nil
}

// DeleteServiceToken deletes a service token by ID.
func (c *Client) DeleteServiceToken(ctx context.Context, id string) error {
	if id == "" {
		return fmt.Errorf("service_token id is required")
	}
	return c.doRequest(ctx, "DELETE", serviceTokenBasePath+"/"+url.PathEscape(id), nil, nil)
}
