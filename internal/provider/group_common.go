package provider

import (
	"context"

	"github.com/braintrustdata/braintrust-sdk-go/openapi"
	"github.com/braintrustdata/terraform-provider-braintrust/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// groupModel is the shared data model for both the group resource and data source.
type groupModel struct {
	ID           types.String `tfsdk:"id"`
	OrgID        types.String `tfsdk:"org_id"`
	Name         types.String `tfsdk:"name"`
	Description  types.String `tfsdk:"description"`
	Created      types.String `tfsdk:"created"`
	DeletedAt    types.String `tfsdk:"deleted_at"`
	UserID       types.String `tfsdk:"user_id"`
	MemberUsers  types.Set    `tfsdk:"member_users"`
	MemberGroups types.Set    `tfsdk:"member_groups"`
}

// mapGroupToModel maps an SDK group to the shared Terraform model.
func mapGroupToModel(group *openapi.Group, model *groupModel) {
	model.ID = types.StringValue(group.Id.String())
	model.OrgID = types.StringValue(group.OrgId.String())
	model.Name = types.StringValue(group.Name)
	model.Description = nullableStringToTF(group.Description)
	model.Created = nullableTimeToTF(group.Created, model.Created)
	model.DeletedAt = nullableTimeToTF(group.DeletedAt, model.DeletedAt)
	model.UserID = nullableUUIDToTF(group.UserId)

	model.MemberUsers = stringSliceToTFSet(groupMemberUsers(group))
	model.MemberGroups = stringSliceToTFSet(client.UUIDStrings(nullableSliceValue(group.MemberGroups)))
}

func groupMemberUsers(group *openapi.Group) []string {
	return client.UUIDStrings(nullableSliceValue(group.MemberUsers))
}

func getGroup(ctx context.Context, c *client.Client, id string) (*openapi.Group, error) {
	resp, err := c.API.GetGroupIdWithResponse(ctx, id)
	if err != nil {
		return nil, err
	}
	return client.JSON(resp.JSON200)
}

func patchGroup(ctx context.Context, c *client.Client, id string, patch openapi.PatchGroup) (*openapi.Group, error) {
	resp, err := client.Write(c.API.PatchGroupIdWithResponse(ctx, id, patch))
	if err != nil {
		return nil, err
	}
	return client.Written(resp.JSON200)
}

// listGroups lists groups by name. The endpoint cannot filter by org_id, so
// that filter is applied here; an empty orgID matches every organization.
func listGroups(ctx context.Context, c *client.Client, name, orgID string) ([]openapi.Group, error) {
	groups, err := client.ListObjects("/v1/group", func(limit int, startingAfter *string) (*client.Page[openapi.Group], error) {
		params := &openapi.GetGroupParams{Limit: &limit, StartingAfter: startingAfter}
		if name != "" {
			params.GroupName = &name
		}
		page, err := c.API.GetGroupWithResponse(ctx, params)
		if err != nil {
			return nil, err
		}
		return page.JSON200, nil
	}, func(group openapi.Group) string { return group.Id.String() })
	if err != nil || orgID == "" {
		return groups, err
	}
	filtered := make([]openapi.Group, 0, len(groups))
	for i := range groups {
		if groups[i].OrgId.String() == orgID {
			filtered = append(filtered, groups[i])
		}
	}
	return filtered, nil
}
