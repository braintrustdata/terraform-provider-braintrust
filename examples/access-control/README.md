# Access Control

Roles, groups, and ACLs for an org with several product teams, an audit function, and an external labeling vendor.

| Group | Role | Granted on |
|---|---|---|
| Admins | built-in `Owner` | Organization |
| Auditors | `Auditor`: read data and ACLs | Every project in the `Team projects` project group |
| `<team> leads` | `Project Lead`: full access, manages ACLs | That team's project |
| `<team> engineers` | `Project Engineer`: create/update, no delete | That team's project |
| Contractors | `External Labeler`: open the project, read datasets and experiments, update experiment scores; no logs or prompts | One team's project |

Teams can't see each other's projects. Add a team by adding an entry to `var.teams`.

## How Braintrust permissions fit together

- A **role** is a named set of permissions, optionally restricted to an object type (`restrict_object_type = "dataset"`).
- A **group** is a set of users (or other groups).
- An **ACL** grants one principal (`group_id` or `user_id`) one role or permission (`role_id` or `permission`) on one object.
- ACLs on `organization` or `org_project` apply to everything beneath them. To grant access to the projects in a project group, use `object_type = "project_group_projects"` with the group's ID; an ACL on `project_group` covers only the group object itself. Adding a project to a project group does not grant anything by itself.
- Permissions restricted to an object type only cover that type. A role restricted to datasets and experiments also needs `read` on `project` for its members to open the project.

## Usage

Every email must belong to an existing member of the org.

```bash
export BRAINTRUST_API_KEY="..."
terraform apply -var-file=team.tfvars
```

```hcl
# team.tfvars
admin_emails      = ["cto@company.com"]
auditor_emails    = ["compliance@company.com"]
contractor_emails = ["labeler@vendor.com"]
labeling_team     = "Chat"
teams = {
  Search = { leads = ["alice@company.com"], engineers = ["bob@company.com"] }
  Chat   = { leads = ["carol@company.com"], engineers = ["dan@company.com", "erin@company.com"] }
}
```
