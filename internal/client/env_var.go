package client

import (
	"context"
	"fmt"
	"net/url"
)

const envVarBasePath = "/v1/env_var"

// EnvVar represents a Braintrust environment variable.
type EnvVar struct {
	ID             string  `json:"id"`
	ObjectType     string  `json:"object_type"`
	ObjectID       string  `json:"object_id"`
	Name           string  `json:"name"`
	Created        *string `json:"created,omitempty"`
	Used           *string `json:"used,omitempty"`
	SecretType     *string `json:"secret_type,omitempty"`
	SecretCategory *string `json:"secret_category,omitempty"`
	// Note: value is write-only. The API never returns it in GET responses.
}

// CreateEnvVarRequest is the request body for creating an env var.
type CreateEnvVarRequest struct {
	ObjectType string  `json:"object_type"`
	ObjectID   string  `json:"object_id"`
	Name       string  `json:"name"`
	Value      *string `json:"value,omitempty"`
	SecretType *string `json:"secret_type,omitempty"`
}

// UpdateEnvVarRequest is the request body for updating an env var.
type UpdateEnvVarRequest struct {
	Name       string  `json:"name"`
	Value      *string `json:"value,omitempty"`
	SecretType *string `json:"secret_type,omitempty"`
}

// ReplaceEnvVarRequest is the request body for PUT (create-or-replace).
type ReplaceEnvVarRequest struct {
	ObjectType string  `json:"object_type"`
	ObjectID   string  `json:"object_id"`
	Name       string  `json:"name"`
	Value      *string `json:"value,omitempty"`
	SecretType *string `json:"secret_type,omitempty"`
}

// CreateEnvVar creates a new env var. Idempotent: returns existing if same name exists.
func (c *Client) CreateEnvVar(ctx context.Context, req CreateEnvVarRequest) (*EnvVar, error) {
	var envVar EnvVar
	if err := c.doRequest(ctx, "POST", envVarBasePath, req, &envVar); err != nil {
		return nil, err
	}
	return &envVar, nil
}

// ReplaceEnvVar creates or replaces an env var by name.
func (c *Client) ReplaceEnvVar(ctx context.Context, req ReplaceEnvVarRequest) (*EnvVar, error) {
	var envVar EnvVar
	if err := c.doRequest(ctx, "PUT", envVarBasePath, req, &envVar); err != nil {
		return nil, err
	}
	return &envVar, nil
}

// GetEnvVar retrieves an env var by ID.
func (c *Client) GetEnvVar(ctx context.Context, id string) (*EnvVar, error) {
	if id == "" {
		return nil, fmt.Errorf("env_var id is required")
	}
	var envVar EnvVar
	if err := c.doRequest(ctx, "GET", envVarBasePath+"/"+url.PathEscape(id), nil, &envVar); err != nil {
		return nil, err
	}
	return &envVar, nil
}

// UpdateEnvVar partially updates an env var by ID.
func (c *Client) UpdateEnvVar(ctx context.Context, id string, req UpdateEnvVarRequest) (*EnvVar, error) {
	if id == "" {
		return nil, fmt.Errorf("env_var id is required")
	}
	var envVar EnvVar
	if err := c.doRequest(ctx, "PATCH", envVarBasePath+"/"+url.PathEscape(id), req, &envVar); err != nil {
		return nil, err
	}
	return &envVar, nil
}

// DeleteEnvVar deletes an env var by ID.
func (c *Client) DeleteEnvVar(ctx context.Context, id string) error {
	if id == "" {
		return fmt.Errorf("env_var id is required")
	}
	return c.doRequest(ctx, "DELETE", envVarBasePath+"/"+url.PathEscape(id), nil, nil)
}
