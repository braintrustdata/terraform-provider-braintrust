package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
)

const projectGroupBasePath = "/v1/project_group"

type ProjectGroup struct {
	ID             string   `json:"id"`
	OrgID          string   `json:"org_id"`
	Name           string   `json:"name"`
	Description    *string  `json:"description"`
	Created        *string  `json:"created"`
	DeletedAt      *string  `json:"deleted_at"`
	UserID         *string  `json:"user_id"`
	MemberProjects []string `json:"member_projects"`
}

type CreateProjectGroupRequest struct {
	OrgID       string  `json:"org_id"`
	Name        string  `json:"name"`
	Description *string `json:"description,omitempty"`
}

type UpdateProjectGroupRequest struct {
	Name                 *string          `json:"name,omitempty"`
	Description          *json.RawMessage `json:"description,omitempty"`
	AddMemberProjects    []string         `json:"add_member_projects,omitempty"`
	RemoveMemberProjects []string         `json:"remove_member_projects,omitempty"`
}

func (c *Client) CreateProjectGroup(ctx context.Context, req CreateProjectGroupRequest) (*ProjectGroup, error) {
	var group ProjectGroup
	if err := c.createNew(ctx, projectGroupBasePath, req, &group, "project group"); err != nil {
		return nil, err
	}
	return &group, nil
}

func (c *Client) GetProjectGroup(ctx context.Context, id string) (*ProjectGroup, error) {
	if id == "" {
		return nil, fmt.Errorf("project group id is required")
	}
	var group ProjectGroup
	if err := c.doRequest(ctx, "GET", projectGroupBasePath+"/"+url.PathEscape(id), nil, &group); err != nil {
		return nil, err
	}
	return &group, nil
}

func (c *Client) ListProjectGroups(ctx context.Context, orgID, name string) ([]ProjectGroup, error) {
	params := url.Values{}
	if name != "" {
		params.Set("project_group_name", name)
	}
	// The API filters by org_name, not org_id. Filter locally after fetching
	// all pages so a group in a later page is not silently missed.
	groups, err := listAllByID(ctx, c, projectGroupBasePath, params, func(group ProjectGroup) string { return group.ID })
	if err != nil {
		return nil, err
	}
	if orgID == "" {
		return groups, nil
	}
	filtered := make([]ProjectGroup, 0, len(groups))
	for _, group := range groups {
		if group.OrgID == orgID {
			filtered = append(filtered, group)
		}
	}
	return filtered, nil
}

func (c *Client) UpdateProjectGroup(ctx context.Context, id string, req UpdateProjectGroupRequest) (*ProjectGroup, error) {
	if id == "" {
		return nil, fmt.Errorf("project group id is required")
	}
	var group ProjectGroup
	if err := c.doRequest(ctx, "PATCH", projectGroupBasePath+"/"+url.PathEscape(id), req, &group); err != nil {
		return nil, err
	}
	return &group, nil
}

func (c *Client) DeleteProjectGroup(ctx context.Context, id string) error {
	if id == "" {
		return fmt.Errorf("project group id is required")
	}
	return c.doRequest(ctx, "DELETE", projectGroupBasePath+"/"+url.PathEscape(id), nil, nil)
}
