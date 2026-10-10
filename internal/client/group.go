package client

import (
	"context"
	"fmt"
	"net/url"
)

const groupBasePath = "/v1/group"

// Group represents a Braintrust group.
type Group struct {
	ID           string   `json:"id"`
	OrgID        string   `json:"org_id"`
	Name         string   `json:"name"`
	Description  *string  `json:"description,omitempty"`
	Created      *string  `json:"created,omitempty"`
	DeletedAt    *string  `json:"deleted_at,omitempty"`
	UserID       *string  `json:"user_id,omitempty"`
	MemberUsers  []string `json:"member_users,omitempty"`
	MemberGroups []string `json:"member_groups,omitempty"`
}

// CreateGroupRequest is the request body for creating a group.
type CreateGroupRequest struct {
	Name         string   `json:"name"`
	Description  *string  `json:"description,omitempty"`
	MemberUsers  []string `json:"member_users,omitempty"`
	MemberGroups []string `json:"member_groups,omitempty"`
}

// UpdateGroupRequest is the request body for updating a group.
// Uses add/remove semantics for members, not full replacement.
type UpdateGroupRequest struct {
	Name               *string  `json:"name,omitempty"`
	Description        *string  `json:"description,omitempty"`
	AddMemberUsers     []string `json:"add_member_users,omitempty"`
	RemoveMemberUsers  []string `json:"remove_member_users,omitempty"`
	AddMemberGroups    []string `json:"add_member_groups,omitempty"`
	RemoveMemberGroups []string `json:"remove_member_groups,omitempty"`
}

// ListGroupsResponse is the response from listing groups.
type ListGroupsResponse struct {
	Objects []Group `json:"objects"`
}

// CreateGroup creates a new group.
func (c *Client) CreateGroup(ctx context.Context, req CreateGroupRequest) (*Group, error) {
	var group Group
	if err := c.doRequest(ctx, "POST", groupBasePath, req, &group); err != nil {
		return nil, err
	}
	return &group, nil
}

// GetGroup retrieves a group by ID.
func (c *Client) GetGroup(ctx context.Context, id string) (*Group, error) {
	if id == "" {
		return nil, fmt.Errorf("group id is required")
	}
	var group Group
	if err := c.doRequest(ctx, "GET", groupBasePath+"/"+url.PathEscape(id), nil, &group); err != nil {
		return nil, err
	}
	return &group, nil
}

// ListGroups lists groups, optionally filtered by name and organization.
func (c *Client) ListGroups(ctx context.Context, name, orgID string) ([]Group, error) {
	path := groupBasePath
	params := url.Values{}
	if name != "" {
		params.Set("group_name", name)
	}
	if orgID != "" {
		params.Set("org_id", orgID)
	}
	if len(params) > 0 {
		path += "?" + params.Encode()
	}
	var resp ListGroupsResponse
	if err := c.doRequest(ctx, "GET", path, nil, &resp); err != nil {
		return nil, err
	}
	return resp.Objects, nil
}

// UpdateGroup updates a group by ID.
func (c *Client) UpdateGroup(ctx context.Context, id string, req *UpdateGroupRequest) (*Group, error) {
	if id == "" {
		return nil, fmt.Errorf("group id is required")
	}
	var group Group
	if err := c.doRequest(ctx, "PATCH", groupBasePath+"/"+url.PathEscape(id), req, &group); err != nil {
		return nil, err
	}
	return &group, nil
}

// DeleteGroup deletes a group by ID.
func (c *Client) DeleteGroup(ctx context.Context, id string) error {
	if id == "" {
		return fmt.Errorf("group id is required")
	}
	return c.doRequest(ctx, "DELETE", groupBasePath+"/"+url.PathEscape(id), nil, nil)
}
