package provider

import (
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

// mapACLToModel maps a client.ACL to the Terraform model.
func mapACLToModel(acl *client.ACL, model *aclModel) {
	model.ID = types.StringValue(acl.ID)
	model.ObjectType = types.StringValue(acl.ObjectType)
	model.ObjectID = types.StringValue(acl.ObjectID)
	model.UserID = stringPtrToTFValue(acl.UserID)
	model.GroupID = stringPtrToTFValue(acl.GroupID)
	model.Permission = stringPtrToTFValue(acl.Permission)
	model.RestrictObjectType = stringPtrToTFValue(acl.RestrictObjectType)
	model.RoleID = stringPtrToTFValue(acl.RoleID)
	model.ObjectOrgID = types.StringValue(acl.ObjectOrgID)
	model.Created = stringPtrToTFValue(acl.Created)
}
