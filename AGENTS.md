# AGENTS.md

This repo contains two programs in one Go module:

- The **Terraform provider** for Braintrust (`main.go`, `internal/provider`): https://registry.terraform.io/providers/braintrustdata/braintrust/latest
- The **`braintrust-terraforming` CLI**, which exports an existing Braintrust org as Terraform config (`cmd/braintrust-terraforming`, `internal/terraforming`).

## How the pieces fit

- Terraform calls `internal/provider`, which calls Braintrust through `internal/client`. That package wraps the Go SDK's `openapi` client, which is pinned to a release tag in `go.mod`.
- The CLI fetches resources through the same `internal/client`. The provider schema is the authority on which attributes the CLI writes, so a new provider attribute shows up in CLI output without changes in `internal/terraforming/schema`.
- Free-form JSON fields (`metadata`, `function_data`, `view_data`, automation and score `config`, ...) are kept as raw JSON (`internal/client/rawjson.go`) instead of being decoded into SDK models, so keys unknown to the pinned SDK still round-trip.
- In `internal/provider`, each type has a `*_resource.go` and/or `*_data_source.go`; the mapping between SDK objects and Terraform state lives in `*_common.go`.
- `tests/offline` runs the CLI against a fake API, then runs `terraform validate`/`plan` on the generated `.tf` with the real provider. Changes to CLI output or provider schemas can break it.

## Adding a resource type

1. `internal/provider`: add `<name>_resource.go`, `<name>_common.go`, and tests; register it in `Resources()` in `provider.go` (and `DataSources()` if there's a data source).
2. `examples/snippets/resources/braintrust_<name>/`: add `resource.tf` (and `import.sh` if importable), then run `make doc`.
3. To export it from the CLI: add a lister in `internal/terraforming/api/resources.go`, a `build<Name>Blocks` function in `internal/terraforming/cmd/generate.go`, and the type to `AllResourceTypes` in `internal/terraforming/cmd/root.go`.

## Commands

Tool versions come from `mise.toml` (run `mise install`). `scripts/check-tool-versions.sh` fails CI if versions pinned elsewhere drift from it.

- `make test`: unit tests plus `tests/offline`. Needs `terraform` on PATH; `go test -short ./...` skips the offline checks.
- `make lint`, `make fmt`
- `make doc`: regenerate `docs/` after changing a resource schema or its examples. CI fails if `docs/` is stale.
- `make check`: runs everything except acceptance tests.
- `make test-acceptance-local`: acceptance tests against a local Braintrust at `http://localhost:8000`. Prefer this. Enterprise-only tests fail against a local stack; that's expected.
- `make test-acceptance`: the same via `scripts/test-acceptance.sh` against `BRAINTRUST_API_URL` (default production) with `BRAINTRUST_API_KEY`; looks up the test org, a user, and (for service-token keys) the service account from the key. Only run against a dedicated test org, never a shared environment.
- `make run-example [MODE=plan|apply|destroy|apply-then-destroy] examples/<name>...`: run `examples/<name>` configs (default `plan`; state in `.example-state/`) against `BRAINTRUST_API_URL` (default production; prefer `http://localhost:8000`). Same test-org rule as acceptance tests.

## Conventions

- Don't edit generated files (`docs/`) by hand.
- Writes that may have committed on the server but returned an error surface as `client.UncertainWriteError`; keep that behavior when adding resources.
- Don't cut releases; maintainers do that through the [release workflow](https://github.com/braintrustdata/terraform-provider-braintrust/actions/workflows/release.yml).
