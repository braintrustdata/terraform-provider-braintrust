package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
)

const promptBasePath = "/v1/prompt"

// Prompt represents a Braintrust prompt.
type Prompt struct {
	ID           string          `json:"id"`
	XactID       string          `json:"_xact_id"`
	ProjectID    string          `json:"project_id"`
	OrgID        string          `json:"org_id"`
	Name         string          `json:"name"`
	Slug         string          `json:"slug"`
	Description  *string         `json:"description,omitempty"`
	Created      *string         `json:"created,omitempty"`
	Tags         []string        `json:"tags,omitempty"`
	FunctionType *string         `json:"function_type,omitempty"`
	PromptData   json.RawMessage `json:"prompt_data,omitempty"`
	Metadata     json.RawMessage `json:"metadata,omitempty"`
}

// CreatePromptRequest is the request body for creating a prompt.
type CreatePromptRequest struct {
	ProjectID    string   `json:"project_id"`
	Name         string   `json:"name"`
	Slug         string   `json:"slug"`
	Description  *string  `json:"description,omitempty"`
	Tags         []string `json:"tags,omitempty"`
	FunctionType *string  `json:"function_type,omitempty"`
	PromptData   any      `json:"prompt_data,omitempty"`
}

// UpdatePromptRequest is the request body for updating a prompt.
type UpdatePromptRequest struct {
	Name        *string   `json:"name,omitempty"`
	Slug        *string   `json:"slug,omitempty"`
	Description *string   `json:"description,omitempty"`
	Tags        *[]string `json:"tags,omitempty"`
	PromptData  any       `json:"prompt_data,omitempty"`
}

// CreatePrompt creates a new prompt.
func (c *Client) CreatePrompt(ctx context.Context, req CreatePromptRequest) (*Prompt, error) {
	var prompt Prompt
	if err := c.createNew(ctx, promptBasePath, req, &prompt, "prompt"); err != nil {
		return nil, err
	}
	return &prompt, nil
}

// GetPrompt retrieves a prompt by ID.
func (c *Client) GetPrompt(ctx context.Context, id string) (*Prompt, error) {
	if id == "" {
		return nil, fmt.Errorf("prompt id is required")
	}
	var prompt Prompt
	if err := c.doRequest(ctx, "GET", promptBasePath+"/"+url.PathEscape(id), nil, &prompt); err != nil {
		return nil, err
	}
	return &prompt, nil
}

// ListPromptsResponse is the response from listing prompts.
type ListPromptsResponse struct {
	Objects []Prompt `json:"objects"`
}

// ListPrompts lists prompts, optionally filtered by name and/or project ID.
func (c *Client) ListPrompts(ctx context.Context, name string, projectID string) ([]Prompt, error) {
	params := url.Values{}
	if name != "" {
		params.Set("prompt_name", name)
	}
	if projectID != "" {
		params.Set("project_id", projectID)
	}
	return listAllByID(ctx, c, promptBasePath, params, func(prompt Prompt) string { return prompt.ID })
}

// UpdatePrompt updates a prompt by ID.
func (c *Client) UpdatePrompt(ctx context.Context, id string, req UpdatePromptRequest) (*Prompt, error) {
	if id == "" {
		return nil, fmt.Errorf("prompt id is required")
	}
	var prompt Prompt
	if err := c.doRequest(ctx, "PATCH", promptBasePath+"/"+url.PathEscape(id), req, &prompt); err != nil {
		return nil, err
	}
	return &prompt, nil
}

// DeletePrompt deletes a prompt by ID.
func (c *Client) DeletePrompt(ctx context.Context, id string) error {
	if id == "" {
		return fmt.Errorf("prompt id is required")
	}
	return c.doRequest(ctx, "DELETE", promptBasePath+"/"+url.PathEscape(id), nil, nil)
}
