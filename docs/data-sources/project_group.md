---
page_title: "braintrust_project_group Data Source - braintrust"
description: |-
  Looks up a Braintrust project group by organization ID and name.
---

# braintrust_project_group (Data Source)

## Schema

### Required

- `org_id` (String) Organization ID.
- `name` (String) Project group name.

### Read-Only

- `id` (String) Project group ID.
- `description` (String) Description.
- `member_projects` (Set of String) Member project IDs.
- `created` (String) Creation timestamp.
- `deleted_at` (String) Deletion timestamp, or null.
- `user_id` (String) Creating user ID.
