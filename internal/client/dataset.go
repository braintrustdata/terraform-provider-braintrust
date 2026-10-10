package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
)

const datasetBasePath = "/v1/dataset"

// Dataset represents a Braintrust dataset.
type Dataset struct {
	ID          string          `json:"id"`
	ProjectID   string          `json:"project_id"`
	Name        string          `json:"name"`
	Description *string         `json:"description,omitempty"`
	Tags        []string        `json:"tags,omitempty"`
	Metadata    json.RawMessage `json:"metadata,omitempty"`
	URLSlug     *string         `json:"url_slug,omitempty"`
	Created     *string         `json:"created,omitempty"`
	DeletedAt   *string         `json:"deleted_at,omitempty"`
	UserID      *string         `json:"user_id,omitempty"`
}

// CreateDatasetRequest is the request body for creating a dataset.
type CreateDatasetRequest struct {
	ProjectID   string          `json:"project_id"`
	Name        string          `json:"name"`
	Description *string         `json:"description,omitempty"`
	Tags        []string        `json:"tags,omitempty"`
	Metadata    json.RawMessage `json:"metadata,omitempty"`
}

// UpdateDatasetRequest is the request body for updating a dataset.
type UpdateDatasetRequest struct {
	Name        *string          `json:"name,omitempty"`
	Description *string          `json:"description,omitempty"`
	Tags        *[]string        `json:"tags,omitempty"`
	Metadata    *json.RawMessage `json:"metadata,omitempty"`
}

// CreateDataset creates a new dataset.
func (c *Client) CreateDataset(ctx context.Context, req CreateDatasetRequest) (*Dataset, error) {
	var dataset Dataset
	if err := c.createNew(ctx, datasetBasePath, req, &dataset, "dataset"); err != nil {
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
	params := url.Values{}
	if name != "" {
		params.Set("dataset_name", name)
	}
	if projectID != "" {
		params.Set("project_id", projectID)
	}
	return listAllByID(ctx, c, datasetBasePath, params, func(dataset Dataset) string { return dataset.ID })
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
