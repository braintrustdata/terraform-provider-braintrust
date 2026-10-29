package provider

import (
	"context"

	"github.com/braintrustdata/braintrust-sdk-go/openapi"
	"github.com/braintrustdata/terraform-provider-braintrust/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// aclModel is the Terraform data model for the ACL resource.
type aclModel struct {
	ID                 types.String `tfsdk:"id"`
	ObjectType         types.String `tfsdk:"object_type"`
	ObjectID           types.String `tfsdk:"object_id"`
	UserID             types.String `tfsdk:"user_id"`
	GroupID            types.String `tfsdk:"group_id"`
	Permission         types.String `tfsdk:"permission"`
	RestrictObjectType types.String `tfsdk:"restrict_object_type"`
	RoleID             types.String `tfsdk:"role_id"`
	ObjectOrgID        types.String `tfsdk:"object_org_id"`
	Created            types.String `tfsdk:"created"`
}

// mapACLToModel maps an SDK ACL to the Terraform model.
func mapACLToModel(acl *openapi.Acl, model *aclModel) {
	model.ID = types.StringValue(acl.Id.String())
	model.ObjectType = types.StringValue(string(acl.ObjectType))
	model.ObjectID = types.StringValue(acl.ObjectId.String())
	model.UserID = nullableUUIDToTF(acl.UserId)
	model.GroupID = nullableUUIDToTF(acl.GroupId)
	model.Permission = enumToTF(acl.Permission)
	model.RestrictObjectType = enumToTF(acl.RestrictObjectType)
	model.RoleID = nullableUUIDToTF(acl.RoleId)
	model.ObjectOrgID = types.StringValue(acl.UnderscoreObjectOrgId.String())
	model.Created = nullableTimeToTF(acl.Created, model.Created)
}

func getACL(ctx context.Context, c *client.Client, id string) (*openapi.Acl, error) {
	resp, err := c.API.GetAclIdWithResponse(ctx, id)
	if err != nil {
		return nil, err
	}
	return client.JSON(resp.JSON200)
}

// listACLs lists the ACLs on plan's object that grant plan's permission or
// role to plan's user or group. The API cannot filter on a null
// restrict_object_type, so results may differ in that field.
func listACLs(ctx context.Context, c *client.Client, plan aclModel) ([]openapi.Acl, error) {
	return client.ListObjects("/v1/acl", func(limit int, startingAfter *string) (*client.Page[openapi.Acl], error) {
		page, err := c.API.GetAclWithResponse(ctx, &openapi.GetAclParams{
			Limit: &limit, StartingAfter: startingAfter,
			ObjectType: openapi.AclObjectType(plan.ObjectType.ValueString()), ObjectId: plan.ObjectID.ValueString(),
			UserId: plan.UserID.ValueStringPointer(), GroupId: plan.GroupID.ValueStringPointer(),
			Permission:         tfStringToEnum[openapi.AclListPermission](plan.Permission),
			RestrictObjectType: tfStringToEnum[openapi.AclListRestrictObjectType](plan.RestrictObjectType),
			RoleId:             plan.RoleID.ValueStringPointer(),
		})
		if err != nil {
			return nil, err
		}
		return page.JSON200, nil
	}, func(acl openapi.Acl) string { return acl.Id.String() })
}

// aclItemFromModel builds the SDK request for an ACL, validating its IDs.
func aclItemFromModel(plan aclModel) (openapi.AclItem, error) {
	item := openapi.AclItem{
		ObjectType:         openapi.AclObjectType(plan.ObjectType.ValueString()),
		Permission:         tfStringToEnum[openapi.Permission](plan.Permission),
		RestrictObjectType: tfStringToEnum[openapi.AclObjectType](plan.RestrictObjectType),
	}
	var err error
	if item.ObjectId, err = parseUUID(plan.ObjectID, "object_id"); err != nil {
		return item, err
	}
	if item.UserId, err = tfStringToNullableUUID(plan.UserID, "user_id"); err != nil {
		return item, err
	}
	if item.GroupId, err = tfStringToNullableUUID(plan.GroupID, "group_id"); err != nil {
		return item, err
	}
	if item.RoleId, err = tfStringToNullableUUID(plan.RoleID, "role_id"); err != nil {
		return item, err
	}
	return item, nil
}
