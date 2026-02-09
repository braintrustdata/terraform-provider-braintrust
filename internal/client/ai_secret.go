package client

import (
	"context"
	"fmt"
	"net/url"
)

const aiSecretBasePath = "/v1/ai_secret"

// AISecret represents a Braintrust AI provider secret (GET response).
type AISecret struct {
	ID            string  `json:"id"`
	Name          string  `json:"name"`
	OrgID         string  `json:"org_id"`
	Type          *string `json:"type,omitempty"`
	PreviewSecret *string `json:"preview_secret,omitempty"`
	Created       *string `json:"created,omitempty"`
	UpdatedAt     *string `json:"updated_at,omitempty"`
}

// CreateAISecretRequest is the request body for creating or replacing an AI secret.
type CreateAISecretRequest struct {
	Name   string  `json:"name"`
	Secret *string `json:"secret,omitempty"`
	Type   *string `json:"type,omitempty"`
}

// UpdateAISecretRequest is the request body for partially updating an AI secret.
type UpdateAISecretRequest struct {
	Name   *string `json:"name,omitempty"`
	Secret *string `json:"secret,omitempty"`
	Type   *string `json:"type,omitempty"`
}

// DeleteAISecretRequest is the request body for deleting an AI secret (by name).
type DeleteAISecretRequest struct {
	Name string `json:"name"`
}

// ReplaceAISecret creates or replaces an AI secret by name (PUT).
func (c *Client) ReplaceAISecret(ctx context.Context, req CreateAISecretRequest) (*AISecret, error) {
	var secret AISecret
	if err := c.doRequest(ctx, "PUT", aiSecretBasePath, req, &secret); err != nil {
		return nil, err
	}
	return &secret, nil
}

// GetAISecret retrieves an AI secret by ID.
func (c *Client) GetAISecret(ctx context.Context, id string) (*AISecret, error) {
	if id == "" {
		return nil, fmt.Errorf("ai_secret id is required")
	}
	var secret AISecret
	if err := c.doRequest(ctx, "GET", aiSecretBasePath+"/"+url.PathEscape(id), nil, &secret); err != nil {
		return nil, err
	}
	return &secret, nil
}

// UpdateAISecret partially updates an AI secret by ID.
func (c *Client) UpdateAISecret(ctx context.Context, id string, req UpdateAISecretRequest) (*AISecret, error) {
	if id == "" {
		return nil, fmt.Errorf("ai_secret id is required")
	}
	var secret AISecret
	if err := c.doRequest(ctx, "PATCH", aiSecretBasePath+"/"+url.PathEscape(id), req, &secret); err != nil {
		return nil, err
	}
	return &secret, nil
}

// DeleteAISecret deletes an AI secret by ID.
func (c *Client) DeleteAISecret(ctx context.Context, id string) error {
	if id == "" {
		return fmt.Errorf("ai_secret id is required")
	}
	return c.doRequest(ctx, "DELETE", aiSecretBasePath+"/"+url.PathEscape(id), nil, nil)
}

// DeleteAISecretByName deletes an AI secret by name (uses the body-based endpoint).
func (c *Client) DeleteAISecretByName(ctx context.Context, name string) error {
	if name == "" {
		return fmt.Errorf("ai_secret name is required")
	}
	req := DeleteAISecretRequest{Name: name}
	return c.doRequest(ctx, "DELETE", aiSecretBasePath, req, nil)
}
