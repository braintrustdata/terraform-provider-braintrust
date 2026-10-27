# CI Pipeline

Braintrust setup for running evals in CI, with separate dev, staging, and prod environments.

```
Push → CI (writer token) → dev or staging project
                           ├── evals run against "Golden Test Cases"
                           ├── experiment scored (Accuracy, Safety) and tagged "ci"
                           └── CI job fails the build if scores regress

Prod logs → online scoring → Accuracy below 0.85 → webhook alert
```

Braintrust log alerts watch project logs, not experiments, so a CI regression has to fail the CI job itself. The alert here covers live production traffic, and assumes an online scoring rule writes the `Accuracy` score to prod logs.

## What it creates

| Per environment | Notes |
|---|---|
| Project | `<product> (dev)`, `(staging)`, `(prod)` |
| Scores and tags | Same `Accuracy`/`Safety` scores and `ci`/`release-candidate` tags everywhere |
| Golden dataset | Created empty; CI loads the regression cases |
| CI writer service account and token | dev and staging only. `CI Writer` role on its own project: push prompts, load dataset rows, record experiments and logs; no delete or access changes |
| Log retention | dev 14 days, staging 30, prod 180 |

A single read-only service account and token (`ci-reader`) gets the built-in `Viewer` role on all three projects, for CI jobs that only report results (PR comments comparing to a baseline, release checks). The prod project also gets the accuracy alert.

CI has no write access to prod. Promoting prompts or data (including prod's golden dataset rows) to prod is a separate, gated release step with its own credentials.

Edit `local.environments` and `local.writer_environments` in `main.tf` to change environments, retention, or where CI can write.

## What Terraform manages, and what CI does

Terraform sets up the environment; your app's CI pipeline fills it in.

| Managed by Terraform (this config) | Done by your app repo's CI, with the dev or staging writer token |
|---|---|
| Projects, scores, tags, empty golden datasets | Load or refresh dataset rows from cases checked into the repo |
| Service accounts, tokens, and their access | Push the prompts and scorers the app ships with |
| Alerts and log retention | Run evals, and fail the build if scores regress |

Dataset rows are data, not configuration, and the provider doesn't manage them. Prompts and scorers can be `braintrust_function` resources, but most teams version them with the app code that uses them and push them from that repo's CI (for example with `bt functions push`), so each change runs through evals before it's promoted. Managing them in Terraform as well would make Terraform report UI and playground edits as drift and revert them. Keep in Terraform only the prompts and scorers that change rarely and are owned by whoever owns this config.

## Usage

Creating service tokens requires an organization-owner service token, so this example uses a second, aliased provider for `braintrust_service_token`. Everything else uses `BRAINTRUST_API_KEY`.

```bash
export BRAINTRUST_API_KEY="..."
terraform apply \
  -var="owner_service_token=..." \
  -var="alert_webhook_url=https://hooks.slack.com/services/..."
```

Then store the outputs in your CI system:

```bash
terraform output -json ci_tokens        # writer tokens for dev and staging
terraform output -raw ci_reader_token   # read-only, all environments
terraform output -json project_ids
```

Token secrets are only available at creation time and are stored in Terraform state. Use an encrypted remote backend.

## Example GitHub Actions job

```yaml
jobs:
  eval:
    runs-on: ubuntu-latest
    environment: staging
    steps:
      - uses: actions/checkout@v4
      - name: Run evals
        env:
          BRAINTRUST_API_KEY: ${{ secrets.BRAINTRUST_CI_TOKEN }}
        run: python run_evals.py --project-id ${{ vars.BRAINTRUST_PROJECT_ID }} --tag ci
```
