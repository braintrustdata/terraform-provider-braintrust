# AGENTS.md

This repo contains two programs in one Go module:

- The **Terraform provider** for Braintrust (`main.go`, `internal/provider`).
- The **`braintrust-terraforming` CLI**, which exports an existing Braintrust org as Terraform config (`cmd/braintrust-terraforming`, `internal/terraforming`).

## Layout

```
main.go                     provider entry point
cmd/braintrust-terraforming CLI entry point (thin; code lives in internal/terraforming)

internal/
  client/        shared API layer: Braintrust Go SDK client plus retries, error types,
                 pagination, and raw-JSON passthrough. Used by both programs.
  provider/      one *_resource.go / *_data_source.go per Terraform type; *_common.go
                 maps between SDK objects and Terraform state
  terraforming/  the CLI
    api/         lists resources from Braintrust via internal/client
    cmd/         commands (generate, genimport, import, version); generate.go builds
                 resource blocks from API objects
    hcl/         writes .tf output; references.go links resources together
    schema/      reads the provider's resource schemas to decide which fields to write

tests/offline/   end-to-end checks: CLI -> fake API -> generated .tf -> terraform
                 validate/plan with the real provider. Runs as part of `go test ./...`.

examples/        sample configs; per-resource examples feed the docs
templates/       doc templates
docs/            generated Registry docs; do not edit by hand
scripts/         install-terraforming.sh, check-tool-versions.sh, test-acceptance.sh
```

## How the pieces fit

- Terraform calls `internal/provider`, which calls Braintrust through `internal/client`. That package wraps the Go SDK's `openapi` client, which is pinned to a release tag in `go.mod`.
- The CLI fetches resources through the same `internal/client`. The provider schema is the authority on which attributes the CLI writes, so a new provider attribute shows up in CLI output without changes in `internal/terraforming/schema`.
- Free-form JSON fields (`metadata`, `function_data`, `view_data`, automation and score `config`, ...) are kept as raw JSON (`internal/client/rawjson.go`) instead of being decoded into SDK models, so keys unknown to the pinned SDK still round-trip.

## Commands

Tool versions come from `mise.toml` (run `mise install`). `scripts/check-tool-versions.sh` fails CI if versions pinned elsewhere drift from it.

- `make test`: unit tests plus `tests/offline`. Needs `terraform` on PATH; `go test -short ./...` skips the offline checks.
- `make lint`, `make fmt`
- `make doc`: regenerate `docs/` after changing a resource schema or its examples. CI fails if `docs/` is stale.
- `make test-acceptance`: acceptance tests (`scripts/test-acceptance.sh`) against `BRAINTRUST_API_URL` (default production) with `BRAINTRUST_API_KEY`; looks up the test org, a user, and (for service-token keys) the service account from the key. Use a local or dedicated test org, never a shared environment.
- `make test-acceptance-local`: the same against `http://localhost:8000`.

## Conventions

- Don't edit generated files (`docs/`) by hand.
- Writes that may have committed on the server but returned an error surface as `client.UncertainWriteError`; keep that behavior when adding resources.
- Releases are cut by pushing a `v*` tag on `main`; `release.yml` re-runs the checks and publishes with GoReleaser.
