package hcl

import (
	"fmt"
	"sort"
)

// ReferenceResolver resolves raw IDs to Terraform resource references
// when linked-resource-types is enabled.
type ReferenceResolver struct {
	// linkedTypes is the set of resource types that should use references.
	linkedTypes map[string]bool

	// idToResourceName maps resource IDs to their Terraform resource names.
	// e.g. "abc-123" → "terraform_managed_resource_abc_123"
	idToResourceName map[string]string

	// idToResourceType maps resource IDs to their Terraform type.
	// e.g. "abc-123" → "braintrust_project"
	idToResourceType map[string]string

	// userIDToEmail maps user IDs to emails for data source references.
	userIDToEmail map[string]string

	// usedUserIDs tracks user IDs actually referenced by emitted groups.
	usedUserIDs       map[string]bool
	roleDataSources   map[string]DataBlock
	groupDataSources  map[string]DataBlock
	usedGroupIDs      map[string]bool
	builtinGroupNames map[string]string
	systemRoleNames   map[string]string
}

// NewReferenceResolver creates a resolver with the given linked types.
func NewReferenceResolver(linkedTypes []string) *ReferenceResolver {
	lt := make(map[string]bool)
	for _, t := range linkedTypes {
		lt[t] = true
	}
	return &ReferenceResolver{
		linkedTypes:       lt,
		idToResourceName:  make(map[string]string),
		idToResourceType:  make(map[string]string),
		userIDToEmail:     make(map[string]string),
		usedUserIDs:       make(map[string]bool),
		roleDataSources:   make(map[string]DataBlock),
		groupDataSources:  make(map[string]DataBlock),
		usedGroupIDs:      make(map[string]bool),
		builtinGroupNames: make(map[string]string),
		systemRoleNames:   make(map[string]string),
	}
}

// Register records a resource's ID → (type, name) mapping.
func (r *ReferenceResolver) Register(id, terraformType, resourceName string) {
	r.idToResourceName[id] = resourceName
	r.idToResourceType[id] = terraformType
}

// RegisterRoleDataSource keeps system roles referenceable without declaring
// them as managed resources.
func (r *ReferenceResolver) RegisterRoleDataSource(id, name string) {
	r.systemRoleNames[id] = name
	resourceName := "role_" + ResourceName(id)
	r.idToResourceName[id] = "data.braintrust_role." + resourceName
	r.idToResourceType[id] = "braintrust_role"
	r.roleDataSources[id] = DataBlock{
		Type: "braintrust_role", Name: resourceName, Comment: name,
		Fields: map[string]any{"name": name},
	}
}

// RegisterGroupDataSource keeps Braintrust's built-in groups referenceable
// without declaring their automatically managed membership as Terraform state.
func (r *ReferenceResolver) RegisterGroupDataSource(id, name, orgID string) {
	r.builtinGroupNames[id] = name
	resourceName := "group_" + ResourceName(id)
	r.idToResourceName[id] = "data.braintrust_group." + resourceName
	r.idToResourceType[id] = "braintrust_group"
	r.groupDataSources[id] = DataBlock{
		Type: "braintrust_group", Name: resourceName, Comment: name,
		Fields: map[string]any{"name": name, "org_id": orgID},
	}
}

func (r *ReferenceResolver) BuiltinGroupName(id string) string {
	return r.builtinGroupNames[id]
}

func (r *ReferenceResolver) SystemRoleName(id string) string {
	return r.systemRoleNames[id]
}

// RegisterUser records a user ID → email mapping for data source lookups.
func (r *ReferenceResolver) RegisterUser(id, email string) {
	r.userIDToEmail[id] = email
}

func (r *ReferenceResolver) UserEmail(id string) string {
	return r.userIDToEmail[id]
}

// Resolve returns a Terraform reference expression for an ID, if the target
// type is in the linked set. Returns empty string if not resolvable.
func (r *ReferenceResolver) Resolve(fieldName, id string) string {
	targetType, ok := r.idToResourceType[id]
	if !ok {
		return ""
	}
	if !r.linkedTypes[targetType] {
		return ""
	}

	name := r.idToResourceName[id]
	if _, external := r.roleDataSources[id]; external {
		return name + ".id"
	}
	if _, external := r.groupDataSources[id]; external {
		r.usedGroupIDs[id] = true
		return name + ".id"
	}
	return fmt.Sprintf("%s.%s.id", targetType, name)
}

func (r *ReferenceResolver) GroupDataSources() []DataBlock {
	ids := make([]string, 0, len(r.usedGroupIDs))
	for id := range r.usedGroupIDs {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	blocks := make([]DataBlock, 0, len(ids))
	for _, id := range ids {
		blocks = append(blocks, r.groupDataSources[id])
	}
	return blocks
}

func (r *ReferenceResolver) RoleDataSources() []DataBlock {
	ids := make([]string, 0, len(r.roleDataSources))
	for id := range r.roleDataSources {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	blocks := make([]DataBlock, 0, len(ids))
	for _, id := range ids {
		blocks = append(blocks, r.roleDataSources[id])
	}
	return blocks
}

// HasLinkedTypes returns true if any linked types are configured.
func (r *ReferenceResolver) HasLinkedTypes() bool {
	return len(r.linkedTypes) > 0
}

// UserRef returns a data-source reference for a user ID, if known.
func (r *ReferenceResolver) UserRef(id string) (string, bool) {
	email, ok := r.userIDToEmail[id]
	if !ok || email == "" {
		return "", false
	}
	r.usedUserIDs[id] = true
	name := "user_" + ResourceName(id)
	return fmt.Sprintf("data.braintrust_user.%s.id", name), true
}

// UserDataSources returns the set of user data source blocks needed
// for user ID references in groups.
func (r *ReferenceResolver) UserDataSources() []DataBlock {
	if len(r.usedUserIDs) == 0 {
		return nil
	}

	ids := make([]string, 0, len(r.usedUserIDs))
	for id := range r.usedUserIDs {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	var blocks []DataBlock
	for _, id := range ids {
		email := r.userIDToEmail[id]
		name := "user_" + ResourceName(id)
		blocks = append(blocks, DataBlock{
			Type:    "braintrust_user",
			Name:    name,
			Comment: email,
			Fields:  map[string]any{"email": email},
		})
	}
	return blocks
}
