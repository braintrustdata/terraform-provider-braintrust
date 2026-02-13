# CI/CD Pipeline Integration

Plug Braintrust into your deployment pipeline with standardized metrics, scoped service tokens, webhook alerting, and golden datasets for regression testing.

## Architecture

```
Code Push → CI Pipeline → Braintrust
                │
                ├── 1. Use ci-runner token to create experiment
                ├── 2. Run evals against golden dataset
                ├── 3. Push scores (Accuracy, Latency, Safety)
                ├── 4. Tag run as "ci" / "nightly" / "release-candidate"
                │
                └── If scores regress:
                    └── Webhook → Slack / PagerDuty
```

## What It Sets Up

### Per-Product Projects

One project per AI product (e.g., Search, Chat, Recommendations). Add products by appending to the `project_names` variable — all standardized infrastructure is applied automatically via `for_each`.

### Standardized Metrics

Every project gets the same scores and tags, making cross-product comparisons meaningful:

| Scores | Tags |
|---|---|
| Accuracy (slider) | `ci` — automated pipeline run |
| Latency (slider) | `nightly` — nightly regression |
| Safety (categorical) | `release-candidate` — RC eval |
| | `baseline` — baseline comparison |

### Golden Datasets

Each project gets a "Golden Test Cases" dataset for regression testing. Populate these with curated examples that your CI pipeline evaluates against on every run.

### Scoped Service Tokens

| Token | Purpose | Scope |
|---|---|---|
| `ci-runner` | Push experiment results from CI | Create experiments + push data |
| `monitoring-readonly` | Read results for dashboards | Read-only |

### Webhook Alerts

Two automations per project fire when scores drop:

| Alert | Threshold | Interval | Use Case |
|---|---|---|---|
| Accuracy Regression | < 0.8 | 5 minutes | Catch quality regressions |
| Safety Violation | < 0.5 | 1 minute | Immediate safety alerts |

### Data Retention

All projects get 60-day log retention to keep storage costs under control while preserving enough history for debugging.

## Resources Created

| Resource | Count | Notes |
|---|---|---|
| Projects | 3 | One per product (configurable) |
| Scores | 9 | 3 scores × 3 projects |
| Tags | 12 | 4 tags × 3 projects |
| Datasets | 3 | Golden test cases per project |
| Service accounts | 2 | CI runner + monitoring identities |
| Service tokens | 2 | CI runner + monitoring |
| Automations | 9 | 2 alerts + 1 retention per project |
| AI secrets | 1 | OpenAI |

## Usage

```bash
export BRAINTRUST_API_KEY="sk-..."

terraform apply \
  -var="openai_api_key=sk-..." \
  -var="slack_webhook_url=https://hooks.slack.com/services/..."
```

After apply, wire the tokens into your CI config:

```bash
# Get the CI runner token (store as CI secret)
terraform output -raw ci_runner_token

# Get project IDs (use in CI scripts to target the right project)
terraform output -json project_ids
```

## Example CI Integration (GitHub Actions)

```yaml
jobs:
  eval:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - name: Run evals
        env:
          BRAINTRUST_API_KEY: ${{ secrets.BRAINTRUST_CI_TOKEN }}
        run: |
          python run_evals.py \
            --project-id ${{ vars.BRAINTRUST_PROJECT_ID }} \
            --dataset "Golden Test Cases" \
            --tag ci
```

## Related Examples

- **Need environment promotion (dev/staging/prod)?** → [multi-environment](../multi-environment/)
- **Need team-level access controls?** → [team-permissions](../team-permissions/)
- **Need a central platform team?** → [platform-team](../platform-team/)
