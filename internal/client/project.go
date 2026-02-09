package client

import (
	"context"
	"fmt"
	"net/url"
)

const projectBasePath = "/v1/project"

// ProjectSettings represents the settings for a Braintrust project.
type ProjectSettings struct {
	ComparisonKey        *string `json:"comparison_key,omitempty"`
	BaselineExperimentID *string `json:"baseline_experiment_id,omitempty"`
}

// Project represents a Braintrust project.
type Project struct {
	ID          string           `json:"id"`
	OrgID       string           `json:"org_id"`
	Name        string           `json:"name"`
	Description *string          `json:"description,omitempty"`
	Created     *string          `json:"created,omitempty"`
	DeletedAt   *string          `json:"deleted_at,omitempty"`
	UserID      *string          `json:"user_id,omitempty"`
	Settings    *ProjectSettings `json:"settings,omitempty"`
}

// CreateProjectRequest is the request body for creating a project.
type CreateProjectRequest struct {
	Name        string  `json:"name"`
	Description *string `json:"description,omitempty"`
	OrgName     *string `json:"org_name,omitempty"`
}

// UpdateProjectRequest is the request body for updating a project.
type UpdateProjectRequest struct {
	Name        *string          `json:"name,omitempty"`
	Description *string          `json:"description,omitempty"`
	Settings    *ProjectSettings `json:"settings,omitempty"`
}

// CreateProject creates a new project.
func (c *Client) CreateProject(ctx context.Context, req CreateProjectRequest) (*Project, error) {
	var project Project
	if err := c.doRequest(ctx, "POST", projectBasePath, req, &project); err != nil {
		return nil, err
	}
	return &project, nil
}

// GetProject retrieves a project by ID.
func (c *Client) GetProject(ctx context.Context, id string) (*Project, error) {
	if id == "" {
		return nil, fmt.Errorf("project id is required")
	}
	var project Project
	if err := c.doRequest(ctx, "GET", projectBasePath+"/"+url.PathEscape(id), nil, &project); err != nil {
		return nil, err
	}
	return &project, nil
}

// UpdateProject updates a project by ID.
func (c *Client) UpdateProject(ctx context.Context, id string, req UpdateProjectRequest) (*Project, error) {
	if id == "" {
		return nil, fmt.Errorf("project id is required")
	}
	var project Project
	if err := c.doRequest(ctx, "PATCH", projectBasePath+"/"+url.PathEscape(id), req, &project); err != nil {
		return nil, err
	}
	return &project, nil
}

// DeleteProject deletes a project by ID.
func (c *Client) DeleteProject(ctx context.Context, id string) error {
	if id == "" {
		return fmt.Errorf("project id is required")
	}
	return c.doRequest(ctx, "DELETE", projectBasePath+"/"+url.PathEscape(id), nil, nil)
}
