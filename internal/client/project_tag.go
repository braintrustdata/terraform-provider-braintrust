package client

import (
	"context"
	"fmt"
	"net/url"
)

const projectTagBasePath = "/v1/project_tag"

// ProjectTag represents a Braintrust project tag.
type ProjectTag struct {
	ID          string  `json:"id"`
	ProjectID   string  `json:"project_id"`
	UserID      *string `json:"user_id,omitempty"`
	Name        string  `json:"name"`
	Description *string `json:"description,omitempty"`
	Color       *string `json:"color,omitempty"`
	Created     *string `json:"created,omitempty"`
}

// CreateProjectTagRequest is the request body for creating a project tag.
type CreateProjectTagRequest struct {
	ProjectID   string  `json:"project_id"`
	Name        string  `json:"name"`
	Description *string `json:"description,omitempty"`
	Color       *string `json:"color,omitempty"`
}

// UpdateProjectTagRequest is the request body for updating a project tag.
type UpdateProjectTagRequest struct {
	Name        *string `json:"name,omitempty"`
	Description *string `json:"description,omitempty"`
	Color       *string `json:"color,omitempty"`
}

// CreateProjectTag creates a new project tag.
func (c *Client) CreateProjectTag(ctx context.Context, req CreateProjectTagRequest) (*ProjectTag, error) {
	var tag ProjectTag
	if err := c.doRequest(ctx, "POST", projectTagBasePath, req, &tag); err != nil {
		return nil, err
	}
	return &tag, nil
}

// GetProjectTag retrieves a project tag by ID.
func (c *Client) GetProjectTag(ctx context.Context, id string) (*ProjectTag, error) {
	if id == "" {
		return nil, fmt.Errorf("project_tag id is required")
	}
	var tag ProjectTag
	if err := c.doRequest(ctx, "GET", projectTagBasePath+"/"+url.PathEscape(id), nil, &tag); err != nil {
		return nil, err
	}
	return &tag, nil
}

// UpdateProjectTag partially updates a project tag by ID.
func (c *Client) UpdateProjectTag(ctx context.Context, id string, req UpdateProjectTagRequest) (*ProjectTag, error) {
	if id == "" {
		return nil, fmt.Errorf("project_tag id is required")
	}
	var tag ProjectTag
	if err := c.doRequest(ctx, "PATCH", projectTagBasePath+"/"+url.PathEscape(id), req, &tag); err != nil {
		return nil, err
	}
	return &tag, nil
}

// DeleteProjectTag deletes a project tag by ID.
func (c *Client) DeleteProjectTag(ctx context.Context, id string) error {
	if id == "" {
		return fmt.Errorf("project_tag id is required")
	}
	return c.doRequest(ctx, "DELETE", projectTagBasePath+"/"+url.PathEscape(id), nil, nil)
}
