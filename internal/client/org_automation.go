package client

import (
	"context"
	"fmt"
	"net/url"
)

const orgAutomationBasePath = "/v1/org_automation"

// OrgRetentionConfig is the only org automation config currently exposed by
// the Braintrust API.
type OrgRetentionConfig struct {
	EventType     string `json:"event_type"`
	ObjectType    string `json:"object_type"`
	RetentionDays int64  `json:"retention_days"`
}

type OrgAutomation struct {
	ID          string             `json:"id"`
	OrgID       string             `json:"org_id"`
	Name        string             `json:"name"`
	Description *string            `json:"description"`
	Config      OrgRetentionConfig `json:"config"`
	UserID      *string            `json:"user_id"`
	Created     *string            `json:"created"`
}

type CreateOrgAutomationRequest struct {
	OrgID       string             `json:"org_id"`
	Name        string             `json:"name"`
	Description *string            `json:"description,omitempty"`
	Config      OrgRetentionConfig `json:"config"`
}

// The nullable description is always sent on update to support clearing it.
type UpdateOrgAutomationRequest struct {
	Name        string             `json:"name"`
	Description *string            `json:"description"`
	Config      OrgRetentionConfig `json:"config"`
}

func (c *Client) CreateOrgAutomation(ctx context.Context, req CreateOrgAutomationRequest) (*OrgAutomation, error) {
	var automation OrgAutomation
	if err := c.createNew(ctx, orgAutomationBasePath, req, &automation, "organization automation"); err != nil {
		return nil, err
	}
	return &automation, nil
}

func (c *Client) GetOrgAutomation(ctx context.Context, id string) (*OrgAutomation, error) {
	if id == "" {
		return nil, fmt.Errorf("organization automation id is required")
	}
	var automation OrgAutomation
	if err := c.doRequest(ctx, "GET", orgAutomationBasePath+"/"+url.PathEscape(id), nil, &automation); err != nil {
		return nil, err
	}
	return &automation, nil
}

func (c *Client) UpdateOrgAutomation(ctx context.Context, id string, req UpdateOrgAutomationRequest) (*OrgAutomation, error) {
	if id == "" {
		return nil, fmt.Errorf("organization automation id is required")
	}
	var automation OrgAutomation
	if err := c.doRequest(ctx, "PATCH", orgAutomationBasePath+"/"+url.PathEscape(id), req, &automation); err != nil {
		return nil, err
	}
	return &automation, nil
}

func (c *Client) DeleteOrgAutomation(ctx context.Context, id string) error {
	if id == "" {
		return fmt.Errorf("organization automation id is required")
	}
	return c.doRequest(ctx, "DELETE", orgAutomationBasePath+"/"+url.PathEscape(id), nil, nil)
}
