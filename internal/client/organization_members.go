package client

import "context"

const organizationMembersPath = "/v1/organization/members"

// ServiceAccountInvite is a service account to create via org-members patch.
type ServiceAccountInvite struct {
	Name string `json:"name"`
}

// InviteUsersRequest contains users/service-accounts to invite.
type InviteUsersRequest struct {
	ServiceAccounts []ServiceAccountInvite `json:"service_accounts,omitempty"`
}

// RemoveUsersRequest contains users/service-accounts to remove.
type RemoveUsersRequest struct {
	IDs []string `json:"ids,omitempty"`
}

// PatchOrganizationMembersRequest modifies organization membership.
type PatchOrganizationMembersRequest struct {
	InviteUsers *InviteUsersRequest `json:"invite_users,omitempty"`
	RemoveUsers *RemoveUsersRequest `json:"remove_users,omitempty"`
}

// AddedUser represents a newly added org member in patch response.
type AddedUser struct {
	ID        string  `json:"id"`
	Email     *string `json:"email,omitempty"`
	APIKey    *string `json:"api_key,omitempty"`
	TokenName *string `json:"token_name,omitempty"`
}

// PatchOrganizationMembersResponse is the response from org-members patch.
type PatchOrganizationMembersResponse struct {
	Status     string      `json:"status"`
	OrgID      string      `json:"org_id"`
	AddedUsers []AddedUser `json:"added_users,omitempty"`
}

// PatchOrganizationMembers modifies organization membership.
func (c *Client) PatchOrganizationMembers(ctx context.Context, req PatchOrganizationMembersRequest) (*PatchOrganizationMembersResponse, error) {
	var resp PatchOrganizationMembersResponse
	if err := c.doRequest(ctx, "PATCH", organizationMembersPath, req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}
