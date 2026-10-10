package client

import (
	"context"
	"fmt"
	"net/url"
)

const mcpServerBasePath = "/v1/mcp_server"

type MCPServer struct {
	ID          string  `json:"id"`
	ProjectID   string  `json:"project_id"`
	Name        string  `json:"name"`
	URL         string  `json:"url"`
	Description *string `json:"description"`
	UserID      *string `json:"user_id"`
	Created     *string `json:"created"`
	DeletedAt   *string `json:"deleted_at"`
}

type CreateMCPServerRequest struct {
	ProjectID   string  `json:"project_id"`
	Name        string  `json:"name"`
	URL         string  `json:"url"`
	Description *string `json:"description,omitempty"`
}

type UpdateMCPServerRequest struct {
	Name        string  `json:"name"`
	URL         string  `json:"url"`
	Description *string `json:"description"`
}

func (c *Client) CreateMCPServer(ctx context.Context, req CreateMCPServerRequest) (*MCPServer, error) {
	var server MCPServer
	if err := c.createNew(ctx, mcpServerBasePath, req, &server, "MCP server"); err != nil {
		return nil, err
	}
	return &server, nil
}

func (c *Client) GetMCPServer(ctx context.Context, id string) (*MCPServer, error) {
	if id == "" {
		return nil, fmt.Errorf("MCP server id is required")
	}
	var server MCPServer
	if err := c.doRequest(ctx, "GET", mcpServerBasePath+"/"+url.PathEscape(id), nil, &server); err != nil {
		return nil, err
	}
	return &server, nil
}

func (c *Client) UpdateMCPServer(ctx context.Context, id string, req UpdateMCPServerRequest) (*MCPServer, error) {
	if id == "" {
		return nil, fmt.Errorf("MCP server id is required")
	}
	var server MCPServer
	if err := c.doRequest(ctx, "PATCH", mcpServerBasePath+"/"+url.PathEscape(id), req, &server); err != nil {
		return nil, err
	}
	return &server, nil
}

func (c *Client) DeleteMCPServer(ctx context.Context, id string) error {
	if id == "" {
		return fmt.Errorf("MCP server id is required")
	}
	return c.doRequest(ctx, "DELETE", mcpServerBasePath+"/"+url.PathEscape(id), nil, nil)
}
