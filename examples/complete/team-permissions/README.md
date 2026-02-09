# Team-Based Access Control Example

This example demonstrates a production-ready multi-team permission model for Braintrust.

## Architecture

```
Organization
├── ML Platform Team
│   ├── Leads      → Project Lead role    → RAG Pipeline, Content Moderation
│   ├── Engineers   → Project Engineer role → RAG Pipeline, Content Moderation
│   └── Viewers     → Project Viewer role   → RAG Pipeline, Content Moderation
│
├── Product AI Team
│   ├── Leads      → Project Lead role    → Customer Support Bot
│   ├── Engineers   → Project Engineer role → Customer Support Bot
│   └── Viewers     → Project Viewer role   → Customer Support Bot
│                                            + Read-only on RAG Pipeline (cross-team)
│
└── Projects (use-case oriented, NOT team-oriented)
    ├── RAG Pipeline          ← owned by ML Platform
    ├── Content Moderation    ← owned by ML Platform
    └── Customer Support Bot  ← owned by Product AI
```

## Key Design Decisions

### Projects = Use Cases, Not Teams

Projects represent what you're building (RAG pipeline, support bot), not who's building it. This keeps things clean when use cases span teams or when team structures change.

### Isolation by Default

We **never** grant permissions at `org_project` level (the "all projects" virtual object). Instead, each team's groups get ACLs scoped to specific `project` IDs. A team with no ACL on a project simply cannot see it.

### Three-Tier Access Within Teams

| Role | Can Read | Can Create/Update | Can Delete | Manage Access |
|---|---|---|---|---|
| **Project Viewer** | Everything | Nothing | Nothing | No |
| **Project Engineer** | Everything | Experiments, datasets, prompts, logs | Nothing | No |
| **Project Lead** | Everything | Everything | Everything | Yes |

The Engineer role uses `restrict_object_type` to grant create/update on specific resource types (experiment, dataset, prompt, project_log) without allowing engineers to modify the project itself or delete anything.

### Cross-Team Visibility

When one team needs read-only access to another team's project, a single ACL binding does the job — no need to restructure groups or roles.

## Permission Inheritance

Braintrust permissions cascade through the object hierarchy:

```
organization → org_project → project → experiment / dataset / prompt / project_log
```

Granting "read" on a **project** automatically grants read on all experiments, datasets, prompts, and logs within it. The `restrict_object_type` field lets you limit a permission to specific child types.

## Usage

```bash
# Set your Braintrust API key
export BRAINTRUST_API_KEY="sk-..."

# Provide the OpenAI key via variable
terraform apply -var="openai_api_key=sk-..."
```

Or use a `terraform.tfvars` file (don't commit it!):

```hcl
openai_api_key = "sk-..."

ml_team_members = {
  leads     = ["alice@company.com"]
  engineers = ["bob@company.com", "carol@company.com"]
  viewers   = ["dave@company.com"]
}

product_team_members = {
  leads     = ["eve@company.com"]
  engineers = ["frank@company.com"]
  viewers   = ["grace@company.com", "heidi@company.com"]
}
```
