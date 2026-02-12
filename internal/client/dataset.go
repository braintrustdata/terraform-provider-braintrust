package client

import (
	"context"
	"fmt"
	"net/url"
)

const datasetBasePath = "/v1/dataset"

// Dataset represents a Braintrust dataset.
type Dataset struct {
	ID          string  `json:"id"`
	ProjectID   string  `json:"project_id"`
	Name        string  `json:"name"`
	Description *string `json:"description,omitempty"`
	Created     *string `json:"created,omitempty"`
	DeletedAt   *string `json:"deleted_at,omitempty"`
	UserID      *string `json:"user_id,omitempty"`
}

// CreateDatasetRequest is the request body for creating a dataset.
type CreateDatasetRequest struct {
	ProjectID   string  `json:"project_id"`
	Name        string  `json:"name"`
	Description *string `json:"description,omitempty"`
}

// UpdateDatasetRequest is the request body for updating a dataset.
type UpdateDatasetRequest struct {
	Name        *string `json:"name,omitempty"`
	Description *string `json:"description,omitempty"`
}

// CreateDataset creates a new dataset.
func (c *Client) CreateDataset(ctx context.Context, req CreateDatasetRequest) (*Dataset, error) {
	var dataset Dataset
	if err := c.doRequest(ctx, "POST", datasetBasePath, req, &dataset); err != nil {
		return nil, err
	}
	return &dataset, nil
}

// GetDataset retrieves a dataset by ID.
func (c *Client) GetDataset(ctx context.Context, id string) (*Dataset, error) {
	if id == "" {
		return nil, fmt.Errorf("dataset id is required")
	}
	var dataset Dataset
	if err := c.doRequest(ctx, "GET", datasetBasePath+"/"+url.PathEscape(id), nil, &dataset); err != nil {
		return nil, err
	}
	return &dataset, nil
}

// ListDatasetsResponse is the response from listing datasets.
type ListDatasetsResponse struct {
	Objects []Dataset `json:"objects"`
}

// ListDatasets lists datasets, optionally filtered by name and/or project ID.
func (c *Client) ListDatasets(ctx context.Context, name string, projectID string) ([]Dataset, error) {
	path := datasetBasePath
	params := url.Values{}
	if name != "" {
		params.Set("dataset_name", name)
	}
	if projectID != "" {
		params.Set("project_id", projectID)
	}
	if len(params) > 0 {
		path += "?" + params.Encode()
	}
	var resp ListDatasetsResponse
	if err := c.doRequest(ctx, "GET", path, nil, &resp); err != nil {
		return nil, err
	}
	return resp.Objects, nil
}

// UpdateDataset updates a dataset by ID.
func (c *Client) UpdateDataset(ctx context.Context, id string, req UpdateDatasetRequest) (*Dataset, error) {
	if id == "" {
		return nil, fmt.Errorf("dataset id is required")
	}
	var dataset Dataset
	if err := c.doRequest(ctx, "PATCH", datasetBasePath+"/"+url.PathEscape(id), req, &dataset); err != nil {
		return nil, err
	}
	return &dataset, nil
}

// DeleteDataset deletes a dataset by ID.
func (c *Client) DeleteDataset(ctx context.Context, id string) error {
	if id == "" {
		return fmt.Errorf("dataset id is required")
	}
	return c.doRequest(ctx, "DELETE", datasetBasePath+"/"+url.PathEscape(id), nil, nil)
}
