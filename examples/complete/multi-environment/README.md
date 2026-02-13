# Multi-Environment Promotion (Dev → Staging → Prod)

Models a deployment pipeline where each environment is a separate Braintrust project with progressively tighter permissions, different alerting thresholds, and environment-appropriate retention policies.

## Architecture

```
Dev (move fast)              Staging (validate)           Prod (protect)
┌─────────────────┐         ┌─────────────────┐         ┌─────────────────┐
│ Engineers: CRUD  │   ──►   │ Engineers: CRU   │   ──►   │ Engineers: R     │
│ Leads: CRUD     │         │ QA: CRU          │         │ SRE: CRUD + ACL │
│                 │         │ Leads: CRUD      │         │ Leads: CRUD+ACL │
│ Alert: <0.5     │         │ Alert: <0.7      │         │ Alert: <0.85    │
│ Retain: 14d     │         │ Retain: 30d      │         │ Retain: 180d    │
└─────────────────┘         └─────────────────┘         └─────────────────┘
```

## Key Design Decisions

### Environments = Projects

Each environment is a separate Braintrust project (e.g., "AI Search — Dev", "AI Search — Staging", "AI Search — Prod"). This gives you hard isolation between environments — there's no way to accidentally write dev data to prod.

### Graduated Permissions

| Group | Dev | Staging | Prod |
|---|---|---|---|
| **Engineers** | Full CRUD | Create/update experiments, datasets, logs (no delete) | Read-only |
| **QA** | — | Same as engineers | — |
| **SRE** | — | — | Full CRUD + ACL management |
| **Tech Leads** | Full CRUD | Full CRUD | Full CRUD + ACL management |

### Stricter Alerting in Production

The same webhook fires for score regressions in all environments, but thresholds tighten and intervals shorten as you move toward production:

| Environment | Accuracy Threshold | Check Interval |
|---|---|---|
| Dev | < 0.5 | 1 hour |
| Staging | < 0.7 | 10 minutes |
| Prod | < 0.85 | 1 minute |

### Longer Retention in Production

Dev data is ephemeral (14 days). Production data is kept much longer for audit and debugging:

| Environment | Retention |
|---|---|
| Dev | 14 days |
| Staging | 30 days |
| Prod | 180 days |

### Per-Environment Service Tokens

Each environment gets its own CI service token, so CI pipelines can only write to the intended environment.

## Resources Created

| Resource | Count | Notes |
|---|---|---|
| Projects | 3 | Dev, Staging, Prod |
| Custom roles | 4 | Dev Engineer, Staging Engineer, Prod Reader, Prod Operator |
| Groups | 4 | Engineers, SRE, QA, Tech Leads |
| ACLs | 9 | 3 environments × 2-3 group bindings each |
| Service accounts | 3 | One CI identity per environment |
| Service tokens | 3 | One per environment |
| Automations | 6 | 3 alerts + 3 retention policies |
| AI secrets | 1 | OpenAI |

## Usage

```bash
export BRAINTRUST_API_KEY="sk-..."

terraform apply \
  -var="openai_api_key=sk-..." \
  -var="slack_webhook_url=https://hooks.slack.com/services/..."
```

The `ci_tokens` output is marked sensitive — use it to configure CI secrets:

```bash
terraform output -json ci_tokens
```

## Related Examples

- **Need team isolation instead of environment isolation?** → [team-permissions](../team-permissions/)
- **Need a central platform team across all environments?** → [platform-team](../platform-team/)
- **Need CI/CD integration without the environment model?** → [cicd-pipeline](../cicd-pipeline/)
