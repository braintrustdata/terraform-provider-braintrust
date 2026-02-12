# External Vendor / Contractor Access

Securely grant limited access to external parties — labeling contractors, consulting partners, and vendor integrations — without exposing sensitive data or intellectual property.

## Architecture

```
Organization
├── Internal Leads
│   └── Project Owner role → Full CRUD + ACL management
│
├── External Labelers
│   └── External Labeler role → Read/update experiments + datasets ONLY
│       ✗ No access to logs (PII)
│       ✗ No access to prompts (IP)
│
├── External Consultants
│   └── External Consultant role → Read experiments + datasets ONLY
│       ✗ No write access
│       ✗ No access to logs or prompts
│
├── Vendor Integration
│   └── Service token → Scoped programmatic access
│
└── Customer Chatbot Project
    ├── Labeling Queue (dataset)
    ├── Golden Test Set (dataset)
    └── Scores: Helpfulness, Safety, Accuracy
```

## Key Design Decisions

### PII Protection

Braintrust logs often contain user inputs (PII). By restricting contractors to `dataset` and `experiment` object types only, log data is never exposed. The `restrict_object_type` field on role permissions makes this airtight.

### IP Protection

Prompts are intellectual property. Both the labeler and consultant roles explicitly exclude `prompt` from their readable object types. Neither external party can see how your models are prompted.

### Separate Groups for Easy Revocation

Each external party gets its own group. To revoke all contractor access, delete the ACL or the group — no need to enumerate individual users. This is critical for offboarding vendors.

### Service Tokens for Machine-to-Machine

Vendor systems that push eval results programmatically use a service token (not a user API key). The token is tied to a service account, making it auditable and revocable independently of any human user.

## Resources Created

| Resource | Count | Notes |
|---|---|---|
| Projects | 1 | Customer Chatbot |
| Custom roles | 3 | External Labeler, External Consultant, Project Owner |
| Groups | 3 | Internal Leads, External Labelers, External Consultants |
| ACLs | 3 | One per group |
| Service tokens | 1 | Vendor integration |
| Datasets | 2 | Labeling Queue, Golden Test Set |
| Scores | 3 | Helpfulness, Safety, Accuracy |
| AI secrets | 1 | OpenAI |
| Automations | 1 | 90-day log retention |

## Usage

```bash
export BRAINTRUST_API_KEY="sk-..."

terraform apply \
  -var="openai_api_key=sk-..." \
  -var="vendor_service_account_id=your-uuid"
```

Or use a `terraform.tfvars`:

```hcl
openai_api_key            = "sk-..."
vendor_service_account_id = "your-service-account-uuid"

labeler_emails    = ["labeler1@contractor.com", "labeler2@contractor.com"]
consultant_emails = ["advisor@consulting.com"]
```

## Prerequisites

The `vendor_service_account_id` must reference an existing service account. Create one in the Braintrust UI under **Organization Settings → Service Accounts**.

## Security Checklist

- [ ] Labelers can only see datasets and experiments (not logs or prompts)
- [ ] Consultants have read-only access (no writes)
- [ ] Vendor service token is stored securely (e.g., secrets manager)
- [ ] External groups are reviewed quarterly and revoked when contracts end

## Related Examples

- **Need internal team structure instead?** → [team-permissions](../team-permissions/)
- **Need compliance/audit controls?** → [compliance-audit](../compliance-audit/)
