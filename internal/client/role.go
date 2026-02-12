package client

import (
	"context"
	"fmt"
	"net/url"
)

const roleBasePath = "/v1/role"

// MemberPermission represents a (permission, restrict_object_type) tuple in a role.
type MemberPermission struct {
	Permission         string  `json:"permission"`
	RestrictObjectType *string `json:"restrict_object_type,omitempty"`
}

// Role represents a Braintrust role.
type Role struct {
	ID                string             `json:"id"`
	OrgID             *string            `json:"org_id,omitempty"`
	Name              string             `json:"name"`
	Description       *string            `json:"description,omitempty"`
	Created           *string            `json:"created,omitempty"`
	DeletedAt         *string            `json:"deleted_at,omitempty"`
	UserID            *string            `json:"user_id,omitempty"`
	MemberPermissions []MemberPermission `json:"member_permissions,omitempty"`
	MemberRoles       []string           `json:"member_roles,omitempty"`
}

// CreateRoleRequest is the request body for creating a role.
type CreateRoleRequest struct {
	Name              string             `json:"name"`
	Description       *string            `json:"description,omitempty"`
	MemberPermissions []MemberPermission `json:"member_permissions,omitempty"`
	MemberRoles       []string           `json:"member_roles,omitempty"`
}

// UpdateRoleRequest is the request body for updating a role.
// Uses add/remove semantics for member_permissions and member_roles.
type UpdateRoleRequest struct {
	Name                    *string            `json:"name,omitempty"`
	Description             *string            `json:"description,omitempty"`
	AddMemberPermissions    []MemberPermission `json:"add_member_permissions,omitempty"`
	RemoveMemberPermissions []MemberPermission `json:"remove_member_permissions,omitempty"`
	AddMemberRoles          []string           `json:"add_member_roles,omitempty"`
	RemoveMemberRoles       []string           `json:"remove_member_roles,omitempty"`
}

// ListRolesResponse is the response from listing roles.
type ListRolesResponse struct {
	Objects []Role `json:"objects"`
}

// CreateRole creates a new role.
func (c *Client) CreateRole(ctx context.Context, req CreateRoleRequest) (*Role, error) {
	var role Role
	if err := c.doRequest(ctx, "POST", roleBasePath, req, &role); err != nil {
		return nil, err
	}
	return &role, nil
}

// GetRole retrieves a role by ID.
func (c *Client) GetRole(ctx context.Context, id string) (*Role, error) {
	if id == "" {
		return nil, fmt.Errorf("role id is required")
	}
	var role Role
	if err := c.doRequest(ctx, "GET", roleBasePath+"/"+url.PathEscape(id), nil, &role); err != nil {
		return nil, err
	}
	return &role, nil
}

// ListRoles lists roles, optionally filtered by name.
func (c *Client) ListRoles(ctx context.Context, name string) ([]Role, error) {
	path := roleBasePath
	if name != "" {
		path += "?role_name=" + url.QueryEscape(name)
	}
	var resp ListRolesResponse
	if err := c.doRequest(ctx, "GET", path, nil, &resp); err != nil {
		return nil, err
	}
	return resp.Objects, nil
}

// UpdateRole updates a role by ID.
func (c *Client) UpdateRole(ctx context.Context, id string, req *UpdateRoleRequest) (*Role, error) {
	if id == "" {
		return nil, fmt.Errorf("role id is required")
	}
	var role Role
	if err := c.doRequest(ctx, "PATCH", roleBasePath+"/"+url.PathEscape(id), req, &role); err != nil {
		return nil, err
	}
	return &role, nil
}

// DeleteRole deletes a role by ID.
func (c *Client) DeleteRole(ctx context.Context, id string) error {
	if id == "" {
		return fmt.Errorf("role id is required")
	}
	return c.doRequest(ctx, "DELETE", roleBasePath+"/"+url.PathEscape(id), nil, nil)
}
