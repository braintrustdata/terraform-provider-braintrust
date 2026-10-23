package provider

import (
	"context"

	"github.com/braintrustdata/braintrust-sdk-go/openapi"
	"github.com/braintrustdata/terraform-provider-braintrust/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type projectGroupModel struct {
	ID             types.String `tfsdk:"id"`
	OrgID          types.String `tfsdk:"org_id"`
	Name           types.String `tfsdk:"name"`
	Description    types.String `tfsdk:"description"`
	Created        types.String `tfsdk:"created"`
	DeletedAt      types.String `tfsdk:"deleted_at"`
	UserID         types.String `tfsdk:"user_id"`
	MemberProjects types.Set    `tfsdk:"member_projects"`
}

func mapProjectGroupToModel(group *openapi.ProjectGroup, model *projectGroupModel) {
	model.ID = types.StringValue(group.Id.String())
	model.OrgID = types.StringValue(group.OrgId.String())
	model.Name = types.StringValue(group.Name)
	model.Description = nullableStringToTF(group.Description)
	model.Created = nullableTimeToTF(group.Created, model.Created)
	model.DeletedAt = nullableTimeToTF(group.DeletedAt, model.DeletedAt)
	model.UserID = nullableUUIDToTF(group.UserId)
	model.MemberProjects = stringSliceToTFSet(client.UUIDStrings(group.MemberProjects))
}

func getProjectGroup(ctx context.Context, c *client.Client, id string) (*openapi.ProjectGroup, error) {
	resp, err := c.API.GetProjectGroupIdWithResponse(ctx, id)
	if err != nil {
		return nil, err
	}
	return client.JSON(resp.JSON200)
}

func patchProjectGroup(ctx context.Context, c *client.Client, id string, patch openapi.PatchProjectGroup) (*openapi.ProjectGroup, error) {
	resp, err := client.Write(c.API.PatchProjectGroupIdWithResponse(ctx, id, patch))
	if err != nil {
		return nil, err
	}
	return client.Written(resp.JSON200)
}

// listProjectGroups lists project groups by name. The endpoint cannot filter
// by org_id, so that filter is applied here; an empty orgID matches all.
func listProjectGroups(ctx context.Context, c *client.Client, orgID, name string) ([]openapi.ProjectGroup, error) {
	groups, err := client.ListObjects("/v1/project_group", func(limit int, startingAfter *string) (*client.Page[openapi.ProjectGroup], error) {
		params := &openapi.GetProjectGroupParams{Limit: &limit, StartingAfter: startingAfter}
		if name != "" {
			params.ProjectGroupName = &name
		}
		page, err := c.API.GetProjectGroupWithResponse(ctx, params)
		if err != nil {
			return nil, err
		}
		return page.JSON200, nil
	}, func(group openapi.ProjectGroup) string { return group.Id.String() })
	if err != nil || orgID == "" {
		return groups, err
	}
	filtered := make([]openapi.ProjectGroup, 0, len(groups))
	for _, group := range groups {
		if group.OrgId.String() == orgID {
			filtered = append(filtered, group)
		}
	}
	return filtered, nil
}
