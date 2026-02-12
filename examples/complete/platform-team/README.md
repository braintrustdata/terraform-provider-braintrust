# Hub-and-Spoke: Central Platform Team + Product Teams

A central ML Platform / MLOps team provides shared infrastructure and governance while product teams independently own their evaluation projects.

## Architecture

```
Organization
├── ML Platform Team (hub)
│   └── Platform Admin role → org_project (ALL projects)
│       Manages: AI credentials, standard scores, tags, retention
│
├── Product Teams (spokes) — isolated from each other
│   ├── Search Team  → Product Engineer → Search project only
│   ├── Chat Team    → Product Engineer → Chat project only
│   └── Recs Team    → Product Engineer → Recommendations project only
│
└── Projects
    ├── Search
    ├── Chat
    └── Recommendations
```

## Key Design Decisions

### Hub: Platform Team Sees Everything

The platform team gets admin access via `org_project` — the "all projects" virtual scope. This means any new project automatically inherits platform team oversight. No per-project ACL maintenance needed.

### Spokes: Product Teams See Only Their Project

Each product team gets a single ACL scoped to `object_type = "project"` with their specific project ID. They cannot see, access, or even know about other teams' projects.

### Standardized Metrics Across All Projects

The platform team enforces consistency by applying the same scores (Accuracy, Latency, Safety, Cost), tags (ci, production, baseline), and retention policies to every project using `for_each`. This makes cross-project comparison meaningful.

### Two Roles, Not Five

Only two custom roles needed:

| Role | Scope | Permissions |
|---|---|---|
| **Platform Admin** | All projects | Full CRUD + ACL management |
| **Product Engineer** | Single project | Full CRUD, no ACL management |

Product engineers get full autonomy within their project (create, read, update, delete experiments/datasets/prompts) but cannot grant access to others or modify the org.

## Resources Created

| Resource | Count | Notes |
|---|---|---|
| Projects | 3 | One per product (Search, Chat, Recs) |
| Custom roles | 2 | Platform Admin, Product Engineer |
| Groups | 4 | Platform + one per product team |
| ACLs | 4 | 1 org-wide (platform) + 3 per-project (teams) |
| Scores | 12 | 4 standard scores × 3 projects |
| Tags | 9 | 3 standard tags × 3 projects |
| Automations | 3 | 90-day retention on each project |
| AI secrets | 1-2 | OpenAI required, Anthropic optional |

## Usage

```bash
export BRAINTRUST_API_KEY="sk-..."

terraform apply -var="openai_api_key=sk-..."
```

Or use a `terraform.tfvars`:

```hcl
openai_api_key = "sk-..."

platform_emails    = ["platform1@company.com", "platform2@company.com"]
search_team_emails = ["search1@company.com", "search2@company.com"]
chat_team_emails   = ["chat1@company.com", "chat2@company.com"]
recs_team_emails   = ["recs1@company.com"]
```

## When to Use This

This pattern works well when:

- You have a dedicated MLOps / platform engineering team.
- Multiple product teams use Braintrust independently.
- You need consistent metrics and governance across all teams.
- Teams should be isolated from each other but the platform team needs cross-cutting visibility.

## Related Examples

- **Need per-team tiered access (viewer/engineer/lead)?** → [team-permissions](../team-permissions/)
- **Need dev/staging/prod separation?** → [multi-environment](../multi-environment/)
