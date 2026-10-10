package client

import (
	"context"
	"fmt"
	"net/url"
)

const aclBasePath = "/v1/acl"

// ACL represents a Braintrust access control list entry.
type ACL struct {
	ID                 string  `json:"id"`
	ObjectType         string  `json:"object_type"`
	ObjectID           string  `json:"object_id"`
	UserID             *string `json:"user_id,omitempty"`
	GroupID            *string `json:"group_id,omitempty"`
	Permission         *string `json:"permission,omitempty"`
	RestrictObjectType *string `json:"restrict_object_type,omitempty"`
	RoleID             *string `json:"role_id,omitempty"`
	ObjectOrgID        string  `json:"_object_org_id"`
	Created            *string `json:"created,omitempty"`
}

// CreateACLRequest is the request body for creating an ACL.
type CreateACLRequest struct {
	ObjectType         string  `json:"object_type"`
	ObjectID           string  `json:"object_id"`
	UserID             *string `json:"user_id,omitempty"`
	GroupID            *string `json:"group_id,omitempty"`
	Permission         *string `json:"permission,omitempty"`
	RestrictObjectType *string `json:"restrict_object_type,omitempty"`
	RoleID             *string `json:"role_id,omitempty"`
}

// CreateACL creates a new ACL entry. This is idempotent -- if an identical
// ACL already exists, the existing one is returned.
func (c *Client) CreateACL(ctx context.Context, req CreateACLRequest) (*ACL, error) {
	var acl ACL
	if err := c.createNew(ctx, aclBasePath, req, &acl, "ACL"); err != nil {
		return nil, err
	}
	return &acl, nil
}

// GetACL retrieves an ACL by ID.
func (c *Client) GetACL(ctx context.Context, id string) (*ACL, error) {
	if id == "" {
		return nil, fmt.Errorf("acl id is required")
	}
	var acl ACL
	if err := c.doRequest(ctx, "GET", aclBasePath+"/"+url.PathEscape(id), nil, &acl); err != nil {
		return nil, err
	}
	return &acl, nil
}

// DeleteACL deletes an ACL by ID.
func (c *Client) DeleteACL(ctx context.Context, id string) error {
	if id == "" {
		return fmt.Errorf("acl id is required")
	}
	return c.doRequest(ctx, "DELETE", aclBasePath+"/"+url.PathEscape(id), nil, nil)
}
