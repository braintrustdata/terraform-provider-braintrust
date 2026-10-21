package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
)

// =============================================================================
// Resource types — matching the Braintrust API response shapes.
// We only include fields relevant to Terraform generation (not write-only).
// =============================================================================

type Project struct {
	ID          string  `json:"id"`
	OrgID       string  `json:"org_id"`
	Name        string  `json:"name"`
	Description *string `json:"description,omitempty"`
	Created     *string `json:"created,omitempty"`
	DeletedAt   *string `json:"deleted_at,omitempty"`
	UserID      *string `json:"user_id,omitempty"`
}

type Organization struct {
	ID string `json:"id"`
}

type Dataset struct {
	ID          string          `json:"id"`
	ProjectID   string          `json:"project_id"`
	Name        string          `json:"name"`
	Description *string         `json:"description,omitempty"`
	Tags        []string        `json:"tags,omitempty"`
	Metadata    json.RawMessage `json:"metadata,omitempty"`
	Created     *string         `json:"created,omitempty"`
	DeletedAt   *string         `json:"deleted_at,omitempty"`
	UserID      *string         `json:"user_id,omitempty"`
}

type Function struct {
	ID             string          `json:"id"`
	ProjectID      string          `json:"project_id"`
	Name           string          `json:"name"`
	Slug           string          `json:"slug"`
	Description    *string         `json:"description,omitempty"`
	Tags           []string        `json:"tags,omitempty"`
	FunctionType   *string         `json:"function_type,omitempty"`
	FunctionData   json.RawMessage `json:"function_data"`
	PromptData     json.RawMessage `json:"prompt_data,omitempty"`
	FunctionSchema json.RawMessage `json:"function_schema,omitempty"`
}

type MemberPermission struct {
	Permission         string  `json:"permission"`
	RestrictObjectType *string `json:"restrict_object_type,omitempty"`
}

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

type ProjectGroup struct {
	ID             string   `json:"id"`
	OrgID          string   `json:"org_id"`
	Name           string   `json:"name"`
	Description    *string  `json:"description"`
	MemberProjects []string `json:"member_projects"`
}

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

type EnvVar struct {
	ID             string  `json:"id"`
	ObjectType     string  `json:"object_type"`
	ObjectID       string  `json:"object_id"`
	Name           string  `json:"name"`
	Created        *string `json:"created,omitempty"`
	Used           *string `json:"used,omitempty"`
	SecretType     *string `json:"secret_type,omitempty"`
	SecretCategory *string `json:"secret_category,omitempty"`
}

type ProjectTag struct {
	ID          string  `json:"id"`
	ProjectID   string  `json:"project_id"`
	Name        string  `json:"name"`
	Description *string `json:"description,omitempty"`
	Color       *string `json:"color,omitempty"`
	Created     *string `json:"created,omitempty"`
	UserID      *string `json:"user_id,omitempty"`
}

type ProjectScore struct {
	ID          string          `json:"id"`
	ProjectID   string          `json:"project_id"`
	Name        string          `json:"name"`
	Description *string         `json:"description,omitempty"`
	ScoreType   string          `json:"score_type"`
	Categories  json.RawMessage `json:"categories,omitempty"`
	Config      json.RawMessage `json:"config,omitempty"`
	Created     *string         `json:"created,omitempty"`
	UserID      *string         `json:"user_id,omitempty"`
}

type View struct {
	ID          string          `json:"id"`
	ObjectType  string          `json:"object_type"`
	ObjectID    string          `json:"object_id"`
	ViewType    string          `json:"view_type"`
	Name        string          `json:"name"`
	Description *string         `json:"description"`
	Starred     *bool           `json:"starred"`
	ViewData    json.RawMessage `json:"view_data"`
	Options     json.RawMessage `json:"options"`
}

type OrgAutomation struct {
	ID          string          `json:"id"`
	OrgID       string          `json:"org_id"`
	Name        string          `json:"name"`
	Description *string         `json:"description"`
	Config      json.RawMessage `json:"config"`
}

type MCPServer struct {
	ID          string  `json:"id"`
	ProjectID   string  `json:"project_id"`
	Name        string  `json:"name"`
	URL         string  `json:"url"`
	Description *string `json:"description"`
	DeletedAt   *string `json:"deleted_at"`
}

type ServiceToken struct {
	ID                  string  `json:"id"`
	Name                string  `json:"name"`
	PreviewName         string  `json:"preview_name"`
	Created             *string `json:"created,omitempty"`
	ServiceAccountID    *string `json:"service_account_id,omitempty"`
	ServiceAccountEmail *string `json:"service_account_email,omitempty"`
	ServiceAccountName  *string `json:"service_account_name,omitempty"`
	OrgID               *string `json:"org_id,omitempty"`
}

type AISecret struct {
	ID            string  `json:"id"`
	Name          string  `json:"name"`
	OrgID         string  `json:"org_id"`
	Type          *string `json:"type,omitempty"`
	PreviewSecret *string `json:"preview_secret,omitempty"`
	Created       *string `json:"created,omitempty"`
	UpdatedAt     *string `json:"updated_at,omitempty"`
}

type ProjectAutomation struct {
	ID          string          `json:"id"`
	ProjectID   string          `json:"project_id"`
	UserID      *string         `json:"user_id,omitempty"`
	Created     *string         `json:"created,omitempty"`
	Name        string          `json:"name"`
	Description *string         `json:"description,omitempty"`
	Config      json.RawMessage `json:"config,omitempty"`
}

type User struct {
	ID         string  `json:"id"`
	GivenName  *string `json:"given_name,omitempty"`
	FamilyName *string `json:"family_name,omitempty"`
	Email      *string `json:"email,omitempty"`
	AvatarURL  *string `json:"avatar_url,omitempty"`
	Created    *string `json:"created,omitempty"`
}

// =============================================================================
// List methods — fetch all objects of a given type.
// =============================================================================

func (c *Client) ListProjects(ctx context.Context) ([]Project, error) {
	raw, err := c.ListAll(ctx, "/v1/project", url.Values{})
	if err != nil {
		return nil, err
	}
	return UnmarshalAll[Project](raw)
}

// GetOrganization verifies that the credential can read the selected org.
func (c *Client) GetOrganization(ctx context.Context, id string) (*Organization, error) {
	params := url.Values{"id": {id}}
	raw, err := c.ListAll(ctx, "/v1/organization", params)
	if err != nil {
		return nil, err
	}
	organizations, err := UnmarshalAll[Organization](raw)
	if err != nil {
		return nil, err
	}
	for _, organization := range organizations {
		if organization.ID == id {
			return &organization, nil
		}
	}
	return nil, fmt.Errorf("organization %s is not readable with this credential", id)
}

func (c *Client) ListDatasets(ctx context.Context, projectID string) ([]Dataset, error) {
	params := url.Values{}
	if projectID != "" {
		params.Set("project_id", projectID)
	}
	raw, err := c.ListAll(ctx, "/v1/dataset", params)
	if err != nil {
		return nil, err
	}
	return UnmarshalAll[Dataset](raw)
}

func (c *Client) ListFunctions(ctx context.Context, projectID string) ([]Function, error) {
	params := url.Values{}
	if projectID != "" {
		params.Set("project_id", projectID)
	}
	raw, err := c.ListAll(ctx, "/v1/function", params)
	if err != nil {
		return nil, err
	}
	return UnmarshalAll[Function](raw)
}

func (c *Client) ListRoles(ctx context.Context) ([]Role, error) {
	raw, err := c.ListAll(ctx, "/v1/role", url.Values{})
	if err != nil {
		return nil, err
	}
	return UnmarshalAll[Role](raw)
}

func (c *Client) ListGroups(ctx context.Context) ([]Group, error) {
	raw, err := c.ListAll(ctx, "/v1/group", url.Values{})
	if err != nil {
		return nil, err
	}
	return UnmarshalAll[Group](raw)
}

func (c *Client) ListProjectGroups(ctx context.Context, orgID string) ([]ProjectGroup, error) {
	raw, err := c.ListAll(ctx, "/v1/project_group", url.Values{"org_id": {orgID}})
	if err != nil {
		return nil, err
	}
	return UnmarshalAll[ProjectGroup](raw)
}

func (c *Client) ListACLs(ctx context.Context, objectType, objectID string) ([]ACL, error) {
	params := url.Values{}
	params.Set("object_type", objectType)
	params.Set("object_id", objectID)
	raw, err := c.ListAll(ctx, "/v1/acl", params)
	if err != nil {
		return nil, err
	}
	return UnmarshalAll[ACL](raw)
}

// ListOrgACLs discovers direct ACLs on every object type in the credential's
// organization, including scopes that are not discoverable from projects.
func (c *Client) ListOrgACLs(ctx context.Context) ([]ACL, error) {
	raw, err := c.ListAll(ctx, "/v1/acl/list_org", url.Values{})
	if err != nil {
		return nil, err
	}
	return UnmarshalAll[ACL](raw)
}

func (c *Client) GetACL(ctx context.Context, id string) (*ACL, error) {
	var acl ACL
	if err := c.doRequest(ctx, "GET", "/v1/acl/"+url.PathEscape(id), nil, &acl); err != nil {
		return nil, err
	}
	return &acl, nil
}

func (c *Client) ListEnvVars(ctx context.Context, objectType, objectID string) ([]EnvVar, error) {
	params := url.Values{}
	if objectType != "" {
		params.Set("object_type", objectType)
	}
	if objectID != "" {
		params.Set("object_id", objectID)
	}
	// Unlike the other list endpoints, /v1/env_var rejects limit and cursor
	// parameters. Its response contains the full set for the requested scope.
	path := "/v1/env_var?" + params.Encode()
	var response listResponse
	err := c.doRequest(ctx, "GET", path, nil, &response)
	if err != nil {
		return nil, err
	}
	return UnmarshalAll[EnvVar](response.Objects)
}

func (c *Client) ListProjectTags(ctx context.Context, projectID string) ([]ProjectTag, error) {
	params := url.Values{}
	if projectID != "" {
		params.Set("project_id", projectID)
	}
	raw, err := c.ListAll(ctx, "/v1/project_tag", params)
	if err != nil {
		return nil, err
	}
	return UnmarshalAll[ProjectTag](raw)
}

func (c *Client) ListProjectScores(ctx context.Context, projectID string) ([]ProjectScore, error) {
	params := url.Values{}
	if projectID != "" {
		params.Set("project_id", projectID)
	}
	raw, err := c.ListAll(ctx, "/v1/project_score", params)
	if err != nil {
		return nil, err
	}
	return UnmarshalAll[ProjectScore](raw)
}

func (c *Client) ListViews(ctx context.Context, objectType, objectID string) ([]View, error) {
	params := url.Values{"object_type": {objectType}, "object_id": {objectID}}
	raw, err := c.ListAll(ctx, "/v1/view", params)
	if err != nil {
		return nil, err
	}
	return UnmarshalAll[View](raw)
}

func (c *Client) ListOrgAutomations(ctx context.Context) ([]OrgAutomation, error) {
	raw, err := c.ListAll(ctx, "/v1/org_automation", url.Values{})
	if err != nil {
		return nil, err
	}
	return UnmarshalAll[OrgAutomation](raw)
}

func (c *Client) ListMCPServers(ctx context.Context) ([]MCPServer, error) {
	raw, err := c.ListAll(ctx, "/v1/mcp_server", url.Values{})
	if err != nil {
		return nil, err
	}
	return UnmarshalAll[MCPServer](raw)
}

func (c *Client) ListServiceTokens(ctx context.Context) ([]ServiceToken, error) {
	raw, err := c.ListAll(ctx, "/v1/service_token", url.Values{})
	if err != nil {
		return nil, err
	}
	return UnmarshalAll[ServiceToken](raw)
}

func (c *Client) ListAISecrets(ctx context.Context) ([]AISecret, error) {
	raw, err := c.ListAll(ctx, "/v1/ai_secret", url.Values{})
	if err != nil {
		return nil, err
	}
	return UnmarshalAll[AISecret](raw)
}

func (c *Client) ListProjectAutomations(ctx context.Context) ([]ProjectAutomation, error) {
	// The public list endpoint does not declare a project_id filter.
	raw, err := c.ListAll(ctx, "/v1/project_automation", url.Values{})
	if err != nil {
		return nil, err
	}
	return UnmarshalAll[ProjectAutomation](raw)
}

func (c *Client) ListUsers(ctx context.Context) ([]User, error) {
	raw, err := c.ListAll(ctx, "/v1/user", url.Values{})
	if err != nil {
		return nil, err
	}
	return UnmarshalAll[User](raw)
}
