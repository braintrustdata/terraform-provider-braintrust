# Compliance & Audit — Separation of Duties

Implements separation-of-duties controls for regulated industries (healthcare, finance, government) where no single role can both build and approve evaluations, and all access is auditable.

## Architecture

```
Organization
├── Compliance Officers    → Read + audit ACLs     → All projects (org_project)
├── Platform Admins        → Full CRUD + ACLs      → All projects (org_project)
│
├── Developers             → Create/update experiments, datasets, logs
│                            ✗ Cannot delete (data preservation)
│                            ✗ Cannot modify prompts (change control)
│                            ✗ Cannot manage access
│
├── Human Reviewers        → Read + update scores/logs (labeling)
│                            ✗ Cannot create or delete
│
├── Red Team               → Create adversarial experiments
│                            ✗ Cannot access prompts (IP)
│                            ✗ Cannot modify existing data (tamper prevention)
│
└── Regulated Projects
    ├── Patient Triage AI
    └── Claims Processing
```

## Compliance Principles

This example follows guidance from **NIST AI RMF** and **ISO/IEC 42001**:

### Separation of Duties

No single role can both create evaluations and approve them. Developers build experiments; reviewers score them; compliance officers audit the process. The roles are mutually exclusive by design.

### Least Privilege

Each role gets exactly the permissions it needs and nothing more:

| Role | Read | Create | Update | Delete | Prompts | ACLs |
|---|---|---|---|---|---|---|
| **Compliance Officer** | All | — | — | — | Yes (read) | Read |
| **Developer** | All | Experiments, datasets, logs | Experiments, datasets, logs | — | — | — |
| **Reviewer** | All | — | Experiments, logs (scoring) | — | Yes (read) | — |
| **Red Team** | Experiments, datasets, logs | Experiments, logs | — | — | — | — |
| **Platform Admin** | All | All | All | All | All | All |

### Audit Trail

Compliance officers can read ACLs (`read_acls` permission) to answer "who has access to what?" at any time. Combined with Braintrust's built-in audit log, this provides full visibility into access patterns.

### Data Preservation

No role except Platform Admin can delete anything. Developers can't destroy data (accidental or malicious), and reviewers can only add scores — not modify the underlying experiments.

### Mandatory Retention

Every regulated project gets both log retention and experiment retention automations. Data cannot be deleted before the retention period expires.

## Resources Created

| Resource | Count | Notes |
|---|---|---|
| Projects | 2 | Patient Triage AI, Claims Processing |
| Custom roles | 5 | Compliance Officer, Developer, Reviewer, Red Team, Platform Admin |
| Groups | 5 | One per role |
| ACLs | 8 | 2 org-wide + 3 roles × 2 projects |
| Automations | 4 | Log retention + experiment retention per project |

## Usage

```bash
export BRAINTRUST_API_KEY="sk-..."

terraform apply
```

Or use a `terraform.tfvars`:

```hcl
compliance_emails = ["compliance@corp.com"]
developer_emails  = ["dev1@corp.com", "dev2@corp.com"]
reviewer_emails   = ["reviewer1@corp.com", "reviewer2@corp.com"]
red_team_emails   = ["redteam@corp.com"]
admin_emails      = ["admin@corp.com"]
retention_days    = 90
```

## Customization

### Adjusting Retention

Change `retention_days` to match your regulatory requirement (e.g., 365 for annual audits, 2555 for 7-year financial record keeping).

### Adding Projects

Add new projects to the `regulated_projects` local and they'll automatically inherit all role bindings and retention policies via `for_each`.

## Related Examples

- **Need external vendor controls?** → [vendor-access](../vendor-access/)
- **Need environment promotion (dev/staging/prod)?** → [multi-environment](../multi-environment/)
