# Startup Quick Start

The simplest useful Braintrust setup for a small team (5-15 people) with one or two AI products. Gets you from zero to a fully configured org in minutes.

## Who This Is For

Early-stage teams where:

- Everyone roughly trusts everyone (no cross-team isolation needed yet).
- A founder/CTO needs full control.
- A handful of engineers build and iterate.
- A product manager reviews results but shouldn't delete anything.

## What It Creates

```
Organization
├── Groups
│   ├── Admins      → Owner (built-in)       → All projects
│   ├── Engineers   → Engineer (built-in)     → All projects
│   └── PMs         → Product Manager (custom) → All projects
│
├── Projects
│   ├── AI Assistant       ← core product
│   └── Internal Tools     ← internal automations
│
└── AI Credentials
    ├── OpenAI
    └── Anthropic (optional)
```

### Resources Created

| Resource | Count | Notes |
|---|---|---|
| Projects | 2 | One per product/use case |
| Custom roles | 1 | Product Manager (read + create experiments) |
| Groups | 3 | Admins, Engineers, PMs |
| ACLs | 3 | Org-wide — no per-project isolation |
| AI secrets | 1-2 | OpenAI required, Anthropic optional |
| Scores | 2 | Quality, Accuracy |
| Tags | 1 | baseline |
| Datasets | 1 | Evaluation Dataset |

## Key Design Decisions

### Org-Wide Permissions (Not Per-Project)

For a small team, all groups are granted access at `org_project` level — meaning permissions apply to every project. This keeps things simple. When you grow past ~15 people and need project isolation, see the [team-permissions](../team-permissions/) or [platform-team](../platform-team/) examples.

### Built-In Roles for Most Users

Only one custom role is needed. The built-in Owner and Engineer roles cover admins and engineers. The custom Product Manager role fills the gap: read everything, create experiments to run evals, but no delete and no infrastructure changes.

### Conditional Anthropic Secret

The Anthropic API key is optional — pass an empty string (the default) to skip it.

## Usage

```bash
export BRAINTRUST_API_KEY="sk-..."

# Minimal — just OpenAI
terraform apply -var="openai_api_key=sk-..."

# With Anthropic
terraform apply \
  -var="openai_api_key=sk-..." \
  -var="anthropic_api_key=sk-ant-..."
```

Or use a `terraform.tfvars` file (don't commit it!):

```hcl
openai_api_key = "sk-..."

admin_emails    = ["cto@startup.com"]
engineer_emails = ["eng1@startup.com", "eng2@startup.com", "eng3@startup.com"]
pm_emails       = ["pm@startup.com"]
```

## Growing Beyond This

When your team outgrows this setup:

- **Need project isolation?** → [team-permissions](../team-permissions/)
- **Need a central platform team?** → [platform-team](../platform-team/)
- **Need dev/staging/prod?** → [multi-environment](../multi-environment/)
- **Need external contractor access?** → [vendor-access](../vendor-access/)
