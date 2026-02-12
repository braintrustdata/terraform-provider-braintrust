# Smoke Test

A minimal but complete integration test that exercises every resource type in the Braintrust Terraform provider. Use this to verify the provider is working end-to-end against your org.

## What It Tests

| Resource Type | What's Verified |
|---|---|
| `braintrust_user` (data) | Email-based user lookup |
| `braintrust_project` | Create with name + description |
| `braintrust_role` | Custom role with multiple `member_permissions` |
| `braintrust_role` (data) | Look up built-in "Viewer" role by name |
| `braintrust_group` | Group with a member, group without members |
| `braintrust_acl` | Role-based ACL, built-in role ACL, direct permission ACL |
| `braintrust_project_score` | Slider type, categorical type |
| `braintrust_project_tag` | Tag with description |
| `braintrust_dataset` | Dataset within a project |
| `braintrust_project_automation` | Webhook alert (logs_config), retention policy |

## Usage

```bash
export BRAINTRUST_API_KEY="your-key"

# Apply — creates all resources
terraform apply -var="my_email=you@company.com"

# Verify outputs
terraform output

# Clean up
terraform destroy -var="my_email=you@company.com"
```

The `my_email` variable must be an email address that already belongs to a member of your Braintrust organization.

## What to Expect

After `terraform apply`, the `summary` output shows the IDs of every created resource:

```
summary = {
  project_id        = "..."
  role_id           = "..."
  group_with_user   = "..."
  group_empty       = "..."
  acl_custom_role   = "..."
  acl_builtin_role  = "..."
  acl_permission    = "..."
  score_slider      = "..."
  score_categorical = "..."
  tag               = "..."
  dataset           = "..."
}
```

If any resource fails to create, the error message will tell you exactly what went wrong (e.g., missing permissions, invalid API key, user not found).

## When to Use This

- **First-time setup**: Verify the provider works against your org before building real infrastructure.
- **Provider upgrades**: Run after upgrading the provider version to catch regressions.
- **CI acceptance tests**: Adapt this as a base for automated acceptance testing.
