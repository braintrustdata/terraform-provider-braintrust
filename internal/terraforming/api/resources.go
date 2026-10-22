package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/braintrustdata/braintrust-sdk-go/openapi"
	"github.com/braintrustdata/terraform-provider-braintrust/internal/client"
	"github.com/google/uuid"
)

// Exported objects whose JSON-encoded attributes are emitted verbatim keep
// those fields as raw JSON instead of round-tripping them through SDK models.
type (
	Dataset           = client.Raw[openapi.Dataset]
	Function          = client.Raw[openapi.Function]
	ProjectScore      = client.Raw[openapi.ProjectScore]
	View              = client.Raw[openapi.View]
	OrgAutomation     = client.Raw[openapi.OrgAutomation]
	ProjectAutomation = client.Raw[openapi.ProjectAutomation]
)

func (c *Client) ListProjects(ctx context.Context) ([]openapi.Project, error) {
	return client.ListAll("/v1/project", func(limit int, startingAfter *string) ([]openapi.Project, error) {
		page, err := c.sdk.API.GetProjectWithResponse(ctx, &openapi.GetProjectParams{Limit: &limit, StartingAfter: startingAfter})
		if err != nil {
			return nil, err
		}
		body, err := client.JSON(page.JSON200)
		if err != nil {
			return nil, err
		}
		return body.Objects, nil
	}, func(p openapi.Project) string { return p.Id.String() })
}

// GetOrganization verifies that the credential can read the selected org.
func (c *Client) GetOrganization(ctx context.Context, id string) error {
	orgID, err := uuid.Parse(id)
	if err != nil {
		return fmt.Errorf("organization ID %q is not a UUID: %w", id, err)
	}
	var ids openapi.Ids
	if err := ids.FromIds0(orgID); err != nil {
		return err
	}
	organizations, err := client.ListAll("/v1/organization", func(limit int, startingAfter *string) ([]openapi.Organization, error) {
		page, err := c.sdk.API.GetOrganizationWithResponse(ctx, &openapi.GetOrganizationParams{Limit: &limit, StartingAfter: startingAfter, Ids: &ids})
		if err != nil {
			return nil, err
		}
		body, err := client.JSON(page.JSON200)
		if err != nil {
			return nil, err
		}
		return body.Objects, nil
	}, func(o openapi.Organization) string { return o.Id.String() })
	if err != nil {
		return err
	}
	for _, organization := range organizations {
		if organization.Id == orgID {
			return nil
		}
	}
	return fmt.Errorf("organization %s is not readable with this credential", id)
}

func (c *Client) ListDatasets(ctx context.Context) ([]Dataset, error) {
	return client.ListAll("/v1/dataset", func(limit int, startingAfter *string) ([]Dataset, error) {
		page, err := c.sdk.API.GetDatasetWithResponse(ctx, &openapi.GetDatasetParams{Limit: &limit, StartingAfter: startingAfter})
		if err != nil {
			return nil, err
		}
		body, err := client.JSON(page.JSON200)
		if err != nil {
			return nil, err
		}
		return client.RawObjects(body.Objects, page.Body)
	}, func(d Dataset) string { return d.Value.Id.String() })
}

func (c *Client) ListFunctions(ctx context.Context) ([]Function, error) {
	return client.ListAll("/v1/function", func(limit int, startingAfter *string) ([]Function, error) {
		page, err := c.sdk.API.GetFunctionWithResponse(ctx, &openapi.GetFunctionParams{Limit: &limit, StartingAfter: startingAfter})
		if err != nil {
			return nil, err
		}
		body, err := client.JSON(page.JSON200)
		if err != nil {
			return nil, err
		}
		return client.RawObjects(body.Objects, page.Body)
	}, func(f Function) string { return f.Value.Id.String() })
}

func (c *Client) ListRoles(ctx context.Context) ([]openapi.Role, error) {
	return client.ListAll("/v1/role", func(limit int, startingAfter *string) ([]openapi.Role, error) {
		page, err := c.sdk.API.GetRoleWithResponse(ctx, &openapi.GetRoleParams{Limit: &limit, StartingAfter: startingAfter})
		if err != nil {
			return nil, err
		}
		body, err := client.JSON(page.JSON200)
		if err != nil {
			return nil, err
		}
		return body.Objects, nil
	}, func(r openapi.Role) string { return r.Id.String() })
}

func (c *Client) ListGroups(ctx context.Context) ([]openapi.Group, error) {
	return client.ListAll("/v1/group", func(limit int, startingAfter *string) ([]openapi.Group, error) {
		page, err := c.sdk.API.GetGroupWithResponse(ctx, &openapi.GetGroupParams{Limit: &limit, StartingAfter: startingAfter})
		if err != nil {
			return nil, err
		}
		body, err := client.JSON(page.JSON200)
		if err != nil {
			return nil, err
		}
		return body.Objects, nil
	}, func(g openapi.Group) string { return g.Id.String() })
}

// ListProjectGroups lists an organization's project groups. The spec does not
// declare the org_id filter, so it is added to each request.
func (c *Client) ListProjectGroups(ctx context.Context, orgID string) ([]openapi.ProjectGroup, error) {
	return client.ListAll("/v1/project_group", func(limit int, startingAfter *string) ([]openapi.ProjectGroup, error) {
		page, err := c.sdk.API.GetProjectGroupWithResponse(ctx, &openapi.GetProjectGroupParams{Limit: &limit, StartingAfter: startingAfter}, client.WithQuery("org_id", orgID))
		if err != nil {
			return nil, err
		}
		body, err := client.JSON(page.JSON200)
		if err != nil {
			return nil, err
		}
		return body.Objects, nil
	}, func(g openapi.ProjectGroup) string { return g.Id.String() })
}

// ListOrgACLs discovers direct ACLs on every object type in the credential's
// organization, including scopes that are not discoverable from projects.
//
// The pinned spec types this response as a bare array, but the API returns
// {"objects": [...]}, so the body is decoded here instead of by the SDK.
func (c *Client) ListOrgACLs(ctx context.Context) ([]openapi.Acl, error) {
	return client.ListAll("/v1/acl/list_org", func(limit int, startingAfter *string) ([]openapi.Acl, error) {
		resp, err := c.sdk.API.AclListOrg(ctx, &openapi.AclListOrgParams{Limit: &limit, StartingAfter: startingAfter})
		if err != nil {
			return nil, err
		}
		defer func() { _ = resp.Body.Close() }()
		raw, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("failed to read response body: %w", err)
		}
		var body struct {
			Objects []openapi.Acl `json:"objects"`
		}
		if err := json.Unmarshal(raw, &body); err != nil {
			return nil, fmt.Errorf("failed to decode ACL list: %w", err)
		}
		return body.Objects, nil
	}, func(a openapi.Acl) string { return a.Id.String() })
}

func (c *Client) GetACL(ctx context.Context, id string) (*openapi.Acl, error) {
	resp, err := c.sdk.API.GetAclIdWithResponse(ctx, id)
	if err != nil {
		return nil, err
	}
	return client.JSON(resp.JSON200)
}

// ListEnvVars lists the env vars in one scope. Unlike the other list
// endpoints, /v1/env_var rejects limit and cursor parameters; its response
// contains the full set for the requested scope.
func (c *Client) ListEnvVars(ctx context.Context, objectType, objectID string) ([]openapi.EnvVar, error) {
	params := &openapi.GetEnvVarParams{}
	if objectType != "" {
		t := openapi.EnvVarObjectType(objectType)
		params.ObjectType = &t
	}
	if objectID != "" {
		params.ObjectId = &objectID
	}
	resp, err := c.sdk.API.GetEnvVarWithResponse(ctx, params)
	if err != nil {
		return nil, err
	}
	body, err := client.JSON(resp.JSON200)
	if err != nil {
		return nil, err
	}
	return body.Objects, nil
}

func (c *Client) ListProjectTags(ctx context.Context) ([]openapi.ProjectTag, error) {
	return client.ListAll("/v1/project_tag", func(limit int, startingAfter *string) ([]openapi.ProjectTag, error) {
		page, err := c.sdk.API.GetProjectTagWithResponse(ctx, &openapi.GetProjectTagParams{Limit: &limit, StartingAfter: startingAfter})
		if err != nil {
			return nil, err
		}
		body, err := client.JSON(page.JSON200)
		if err != nil {
			return nil, err
		}
		return body.Objects, nil
	}, func(t openapi.ProjectTag) string { return t.Id.String() })
}

func (c *Client) ListProjectScores(ctx context.Context) ([]ProjectScore, error) {
	return client.ListAll("/v1/project_score", func(limit int, startingAfter *string) ([]ProjectScore, error) {
		page, err := c.sdk.API.GetProjectScoreWithResponse(ctx, &openapi.GetProjectScoreParams{Limit: &limit, StartingAfter: startingAfter})
		if err != nil {
			return nil, err
		}
		body, err := client.JSON(page.JSON200)
		if err != nil {
			return nil, err
		}
		return client.RawObjects(body.Objects, page.Body)
	}, func(s ProjectScore) string { return s.Value.Id.String() })
}

func (c *Client) ListViews(ctx context.Context, objectType, objectID string) ([]View, error) {
	return client.ListAll("/v1/view", func(limit int, startingAfter *string) ([]View, error) {
		page, err := c.sdk.API.GetViewWithResponse(ctx, &openapi.GetViewParams{
			Limit: &limit, StartingAfter: startingAfter,
			ObjectType: openapi.AclObjectType(objectType), ObjectId: objectID,
		})
		if err != nil {
			return nil, err
		}
		body, err := client.JSON(page.JSON200)
		if err != nil {
			return nil, err
		}
		return client.RawObjects(body.Objects, page.Body)
	}, func(v View) string { return v.Value.Id.String() })
}

func (c *Client) ListOrgAutomations(ctx context.Context) ([]OrgAutomation, error) {
	return client.ListAll("/v1/org_automation", func(limit int, startingAfter *string) ([]OrgAutomation, error) {
		page, err := c.sdk.API.GetOrgAutomationWithResponse(ctx, &openapi.GetOrgAutomationParams{Limit: &limit, StartingAfter: startingAfter})
		if err != nil {
			return nil, err
		}
		body, err := client.JSON(page.JSON200)
		if err != nil {
			return nil, err
		}
		return client.RawObjects(body.Objects, page.Body)
	}, func(a OrgAutomation) string { return a.Value.Id.String() })
}

func (c *Client) ListMCPServers(ctx context.Context) ([]openapi.MCPServer, error) {
	return client.ListAll("/v1/mcp_server", func(limit int, startingAfter *string) ([]openapi.MCPServer, error) {
		page, err := c.sdk.API.GetMcpServerWithResponse(ctx, &openapi.GetMcpServerParams{Limit: &limit, StartingAfter: startingAfter})
		if err != nil {
			return nil, err
		}
		body, err := client.JSON(page.JSON200)
		if err != nil {
			return nil, err
		}
		return body.Objects, nil
	}, func(s openapi.MCPServer) string { return s.Id.String() })
}

func (c *Client) ListServiceTokens(ctx context.Context) ([]openapi.ServiceToken, error) {
	return client.ListAll("/v1/service_token", func(limit int, startingAfter *string) ([]openapi.ServiceToken, error) {
		page, err := c.sdk.API.GetServiceTokenWithResponse(ctx, &openapi.GetServiceTokenParams{Limit: &limit, StartingAfter: startingAfter})
		if err != nil {
			return nil, err
		}
		body, err := client.JSON(page.JSON200)
		if err != nil {
			return nil, err
		}
		return body.Objects, nil
	}, func(t openapi.ServiceToken) string { return t.Id.String() })
}

func (c *Client) ListAISecrets(ctx context.Context) ([]openapi.AISecret, error) {
	return client.ListAll("/v1/ai_secret", func(limit int, startingAfter *string) ([]openapi.AISecret, error) {
		page, err := c.sdk.API.GetAiSecretWithResponse(ctx, &openapi.GetAiSecretParams{Limit: &limit, StartingAfter: startingAfter})
		if err != nil {
			return nil, err
		}
		body, err := client.JSON(page.JSON200)
		if err != nil {
			return nil, err
		}
		return body.Objects, nil
	}, func(s openapi.AISecret) string { return s.Id.String() })
}

// ListProjectAutomations lists every visible project automation; the list
// endpoint does not declare a project_id filter.
func (c *Client) ListProjectAutomations(ctx context.Context) ([]ProjectAutomation, error) {
	return client.ListAll("/v1/project_automation", func(limit int, startingAfter *string) ([]ProjectAutomation, error) {
		page, err := c.sdk.API.GetProjectAutomationWithResponse(ctx, &openapi.GetProjectAutomationParams{Limit: &limit, StartingAfter: startingAfter})
		if err != nil {
			return nil, err
		}
		body, err := client.JSON(page.JSON200)
		if err != nil {
			return nil, err
		}
		return client.RawObjects(body.Objects, page.Body)
	}, func(a ProjectAutomation) string { return a.Value.Id.String() })
}

func (c *Client) ListUsers(ctx context.Context) ([]openapi.User, error) {
	return client.ListAll("/v1/user", func(limit int, startingAfter *string) ([]openapi.User, error) {
		page, err := c.sdk.API.GetUserWithResponse(ctx, &openapi.GetUserParams{Limit: &limit, StartingAfter: startingAfter})
		if err != nil {
			return nil, err
		}
		body, err := client.JSON(page.JSON200)
		if err != nil {
			return nil, err
		}
		return body.Objects, nil
	}, func(u openapi.User) string { return u.Id.String() })
}
