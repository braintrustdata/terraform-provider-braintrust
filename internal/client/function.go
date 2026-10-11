package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
)

const functionBasePath = "/v1/function"

// Function is a saved Braintrust function, including scorers and preprocessors.
type Function struct {
	ID             string          `json:"id"`
	XactID         string          `json:"_xact_id"`
	ProjectID      string          `json:"project_id"`
	OrgID          string          `json:"org_id"`
	Name           string          `json:"name"`
	Slug           string          `json:"slug"`
	Description    *string         `json:"description,omitempty"`
	Created        *string         `json:"created,omitempty"`
	Tags           []string        `json:"tags,omitempty"`
	FunctionType   *string         `json:"function_type,omitempty"`
	FunctionData   json.RawMessage `json:"function_data"`
	PromptData     json.RawMessage `json:"prompt_data,omitempty"`
	FunctionSchema json.RawMessage `json:"function_schema,omitempty"`
	Metadata       json.RawMessage `json:"metadata,omitempty"`
}

type CreateFunctionRequest struct {
	ProjectID      string   `json:"project_id"`
	Name           string   `json:"name"`
	Slug           string   `json:"slug"`
	FunctionType   *string  `json:"function_type,omitempty"`
	FunctionData   any      `json:"function_data"`
	Description    *string  `json:"description,omitempty"`
	Tags           []string `json:"tags,omitempty"`
	PromptData     any      `json:"prompt_data,omitempty"`
	FunctionSchema any      `json:"function_schema,omitempty"`
}

type UpdateFunctionRequest struct {
	Name         *string   `json:"name,omitempty"`
	Description  *string   `json:"description,omitempty"`
	Tags         *[]string `json:"tags,omitempty"`
	FunctionData any       `json:"function_data,omitempty"`
	PromptData   any       `json:"prompt_data,omitempty"`
}

func (c *Client) CreateFunction(ctx context.Context, req CreateFunctionRequest) (*Function, error) {
	var function Function
	if err := c.createNew(ctx, functionBasePath, req, &function, "function"); err != nil {
		return nil, err
	}
	return &function, nil
}

func (c *Client) GetFunction(ctx context.Context, id string) (*Function, error) {
	if id == "" {
		return nil, fmt.Errorf("function id is required")
	}
	var function Function
	if err := c.doRequest(ctx, "GET", functionBasePath+"/"+url.PathEscape(id), nil, &function); err != nil {
		return nil, err
	}
	return &function, nil
}

func (c *Client) ListFunctions(ctx context.Context, name, projectID string) ([]Function, error) {
	params := url.Values{}
	if name != "" {
		params.Set("function_name", name)
	}
	if projectID != "" {
		params.Set("project_id", projectID)
	}
	return listAllByID(ctx, c, functionBasePath, params, func(function Function) string { return function.ID })
}

func (c *Client) UpdateFunction(ctx context.Context, id string, req UpdateFunctionRequest) (*Function, error) {
	if id == "" {
		return nil, fmt.Errorf("function id is required")
	}
	var function Function
	if err := c.doRequest(ctx, "PATCH", functionBasePath+"/"+url.PathEscape(id), req, &function); err != nil {
		return nil, err
	}
	return &function, nil
}

func (c *Client) DeleteFunction(ctx context.Context, id string) error {
	if id == "" {
		return fmt.Errorf("function id is required")
	}
	return c.doRequest(ctx, "DELETE", functionBasePath+"/"+url.PathEscape(id), nil, nil)
}
