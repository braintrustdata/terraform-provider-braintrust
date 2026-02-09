package client

import (
	"context"
	"net/url"
)

const userBasePath = "/v1/user"

// User represents a Braintrust user.
type User struct {
	ID         string  `json:"id"`
	GivenName  *string `json:"given_name,omitempty"`
	FamilyName *string `json:"family_name,omitempty"`
	Email      *string `json:"email,omitempty"`
	AvatarURL  *string `json:"avatar_url,omitempty"`
	Created    *string `json:"created,omitempty"`
}

// ListUsersResponse is the response from the list users endpoint.
type ListUsersResponse struct {
	Objects []User `json:"objects"`
}

// ListUsersByEmail lists users filtered by email address.
func (c *Client) ListUsersByEmail(ctx context.Context, email string) ([]User, error) {
	path := userBasePath
	if email != "" {
		path += "?email=" + url.QueryEscape(email)
	}
	var resp ListUsersResponse
	if err := c.doRequest(ctx, "GET", path, nil, &resp); err != nil {
		return nil, err
	}
	return resp.Objects, nil
}

// ListUsersByIDs lists users filtered by IDs.
func (c *Client) ListUsersByIDs(ctx context.Context, ids []string) ([]User, error) {
	path := userBasePath + "?"
	for i, id := range ids {
		if i > 0 {
			path += "&"
		}
		path += "ids=" + url.QueryEscape(id)
	}
	var resp ListUsersResponse
	if err := c.doRequest(ctx, "GET", path, nil, &resp); err != nil {
		return nil, err
	}
	return resp.Objects, nil
}
