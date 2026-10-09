---
page_title: "braintrust_project_group_membership Resource - braintrust"
description: |-
  Manages a project's membership in a Braintrust project group.
---

# braintrust_project_group_membership (Resource)

Manages one relationship between a project and a project group. Use a Terraform reference for both IDs when Terraform manages both objects. Terraform then removes membership before destroying the project or group. A project may have multiple membership resources for different groups.

Do not also set `member_projects` on the same `braintrust_project_group` resource. The API requires `create_acls` on the project to add it and `delete_acls` to remove it.

Import with `project_group_id/project_id`.

## Schema

### Required

- `project_group_id` (String) Project group ID. Changing it replaces membership.
- `project_id` (String) Project ID. Changing it replaces membership.

### Read-Only

- `id` (String) Composite ID in `project_group_id/project_id` form.
