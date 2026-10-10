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
	params := url.Values{}
	if email != "" {
		params.Set("email", email)
	}
	return listAllByID(ctx, c, userBasePath, params, func(user User) string { return user.ID })
}

// ListUsersByIDs lists users filtered by IDs.
func (c *Client) ListUsersByIDs(ctx context.Context, ids []string) ([]User, error) {
	params := url.Values{}
	for _, id := range ids {
		params.Add("ids", id)
	}
	return listAllByID(ctx, c, userBasePath, params, func(user User) string { return user.ID })
}
