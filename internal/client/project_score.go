package client

import (
	"context"
	"fmt"
	"net/url"
)

const projectScoreBasePath = "/v1/project_score"

// ProjectScore represents a Braintrust project score.
type ProjectScore struct {
	ID          string  `json:"id"`
	ProjectID   string  `json:"project_id"`
	UserID      *string `json:"user_id,omitempty"`
	Name        string  `json:"name"`
	Description *string `json:"description,omitempty"`
	ScoreType   string  `json:"score_type"`
	Created     *string `json:"created,omitempty"`
}

// CreateProjectScoreRequest is the request body for creating a project score.
type CreateProjectScoreRequest struct {
	ProjectID   string  `json:"project_id"`
	Name        string  `json:"name"`
	Description *string `json:"description,omitempty"`
	ScoreType   string  `json:"score_type"`
}

// UpdateProjectScoreRequest is the request body for updating a project score.
type UpdateProjectScoreRequest struct {
	Name        *string `json:"name,omitempty"`
	Description *string `json:"description,omitempty"`
	ScoreType   *string `json:"score_type,omitempty"`
}

// CreateProjectScore creates a new project score.
func (c *Client) CreateProjectScore(ctx context.Context, req CreateProjectScoreRequest) (*ProjectScore, error) {
	var score ProjectScore
	if err := c.doRequest(ctx, "POST", projectScoreBasePath, req, &score); err != nil {
		return nil, err
	}
	return &score, nil
}

// GetProjectScore retrieves a project score by ID.
func (c *Client) GetProjectScore(ctx context.Context, id string) (*ProjectScore, error) {
	if id == "" {
		return nil, fmt.Errorf("project_score id is required")
	}
	var score ProjectScore
	if err := c.doRequest(ctx, "GET", projectScoreBasePath+"/"+url.PathEscape(id), nil, &score); err != nil {
		return nil, err
	}
	return &score, nil
}

// UpdateProjectScore partially updates a project score by ID.
func (c *Client) UpdateProjectScore(ctx context.Context, id string, req UpdateProjectScoreRequest) (*ProjectScore, error) {
	if id == "" {
		return nil, fmt.Errorf("project_score id is required")
	}
	var score ProjectScore
	if err := c.doRequest(ctx, "PATCH", projectScoreBasePath+"/"+url.PathEscape(id), req, &score); err != nil {
		return nil, err
	}
	return &score, nil
}

// DeleteProjectScore deletes a project score by ID.
func (c *Client) DeleteProjectScore(ctx context.Context, id string) error {
	if id == "" {
		return fmt.Errorf("project_score id is required")
	}
	return c.doRequest(ctx, "DELETE", projectScoreBasePath+"/"+url.PathEscape(id), nil, nil)
}
