package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
)

const projectAutomationBasePath = "/v1/project_automation"

// ProjectAutomation represents a Braintrust project automation.
type ProjectAutomation struct {
	ID          string          `json:"id"`
	ProjectID   string          `json:"project_id"`
	UserID      *string         `json:"user_id,omitempty"`
	Created     *string         `json:"created,omitempty"`
	Name        string          `json:"name"`
	Description *string         `json:"description,omitempty"`
	Config      json.RawMessage `json:"config,omitempty"`
}

// CreateProjectAutomationRequest is the request body for creating a project automation.
type CreateProjectAutomationRequest struct {
	ProjectID   string          `json:"project_id"`
	Name        string          `json:"name"`
	Description *string         `json:"description,omitempty"`
	Config      json.RawMessage `json:"config"`
}

// UpdateProjectAutomationRequest is the request body for updating a project automation.
type UpdateProjectAutomationRequest struct {
	Name        *string         `json:"name,omitempty"`
	Description *string         `json:"description,omitempty"`
	Config      json.RawMessage `json:"config,omitempty"`
}

// CreateProjectAutomation creates a new project automation.
func (c *Client) CreateProjectAutomation(ctx context.Context, req CreateProjectAutomationRequest) (*ProjectAutomation, error) {
	var automation ProjectAutomation
	if err := c.createNew(ctx, projectAutomationBasePath, req, &automation, "project automation"); err != nil {
		return nil, err
	}
	return &automation, nil
}

// GetProjectAutomation retrieves a project automation by ID.
func (c *Client) GetProjectAutomation(ctx context.Context, id string) (*ProjectAutomation, error) {
	if id == "" {
		return nil, fmt.Errorf("project_automation id is required")
	}
	var automation ProjectAutomation
	if err := c.doRequest(ctx, "GET", projectAutomationBasePath+"/"+url.PathEscape(id), nil, &automation); err != nil {
		return nil, err
	}
	return &automation, nil
}

// UpdateProjectAutomation updates a project automation by ID.
func (c *Client) UpdateProjectAutomation(ctx context.Context, id string, req UpdateProjectAutomationRequest) (*ProjectAutomation, error) {
	if id == "" {
		return nil, fmt.Errorf("project_automation id is required")
	}
	var automation ProjectAutomation
	if err := c.doRequest(ctx, "PATCH", projectAutomationBasePath+"/"+url.PathEscape(id), req, &automation); err != nil {
		return nil, err
	}
	return &automation, nil
}

// DeleteProjectAutomation deletes a project automation by ID.
func (c *Client) DeleteProjectAutomation(ctx context.Context, id string) error {
	if id == "" {
		return fmt.Errorf("project_automation id is required")
	}
	return c.doRequest(ctx, "DELETE", projectAutomationBasePath+"/"+url.PathEscape(id), nil, nil)
}
