---
page_title: "braintrust_project_group Resource - braintrust"
description: |-
  Manages a Braintrust project group.
---

# braintrust_project_group (Resource)

Manages a project group. A project can belong to multiple project groups, and access granted to a project group can apply to its member projects.

For projects also managed by Terraform, leave `member_projects` unset and manage each relationship with `braintrust_project_group_membership`. This gives Terraform a dependency on both the group and project, so it removes membership before destroying either one. Use `member_projects` only when you can remove memberships in a separate apply before destroying projects. Do not use both approaches for the same group.

The API requires `create_acls` on projects being added and `delete_acls` on projects being removed. Deleting a project group requires `delete_acls` on its active member projects. A project group cannot contain other project groups.

Import with the project group ID.

## Schema

### Required

- `org_id` (String) Organization ID. Changing it replaces the group.
- `name` (String) Name, unique within the organization.

### Optional

- `description` (String) Description.
- `member_projects` (Set of String) Project IDs. Use this only when separate membership resources are not used for this group.

### Read-Only

- `id` (String) Project group ID.
- `created` (String) Creation timestamp.
- `deleted_at` (String) Deletion timestamp, or null.
- `user_id` (String) Creating user ID.
