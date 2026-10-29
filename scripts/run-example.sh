#!/usr/bin/env sh

# Runs an examples/<name> configuration against the API at BRAINTRUST_API_URL
# (default https://api.braintrust.dev) with a locally built provider.
#
#   scripts/run-example.sh examples/<name> [MODE]
#
# MODE is one of:
#   plan                show what apply would change (default)
#   apply               apply, check that a second plan is empty, keep the resources
#   destroy             destroy resources left by apply
#   apply-then-destroy  apply, check that a second plan is empty, then destroy
#
# State is kept in .example-state/<name>/ (gitignored) between runs, along with
# the API URL it belongs to; runs against a different URL are refused.
#
# Example variables are filled from the key: emails resolve to the org's first
# human member, and owner_service_token reuses the key when it is a service
# token (set BRAINTRUST_OWNER_SERVICE_TOKEN otherwise). Any TF_VAR_* already in
# the environment wins. Examples use fixed resource names and overwrite AI
# secrets and env vars of the same name, so only use a dedicated test org.

set -eu

cd "$(dirname "$0")/.."

usage="usage: $0 <$(dirname examples/*/main.tf | tr '\n' '|' | sed 's/|$//')> [plan|apply|destroy|apply-then-destroy]"
name=${1:?$usage}
name=${name%/}
name=${name#examples/}
mode=${2:-plan}
src="examples/$name"
[ -f "$src/main.tf" ] || { echo "$usage" >&2; exit 2; }
case "$mode" in plan | apply | destroy | apply-then-destroy) ;; *) echo "$usage" >&2; exit 2 ;; esac

: "${BRAINTRUST_API_KEY:?BRAINTRUST_API_KEY must be set}"
: "${BRAINTRUST_API_URL:=https://api.braintrust.dev}"
export BRAINTRUST_API_URL

state=".example-state/$name"
if [ -f "$state/api_url" ] && [ "$(cat "$state/api_url")" != "$BRAINTRUST_API_URL" ]; then
  echo "$state belongs to $(cat "$state/api_url"), not $BRAINTRUST_API_URL" >&2
  exit 1
fi
if [ "$mode" = destroy ] && [ ! -f "$state/terraform.tfstate" ]; then
  echo "nothing to destroy: no state in $state" >&2
  exit 1
fi

. scripts/braintrust-key.sh

# The provider uses the key's org, so the emails below must come from it too.
key_org_id "use a key for a single test org" >/dev/null
email=$(key_first_user | jq -er .email)
if [ -z "${BRAINTRUST_OWNER_SERVICE_TOKEN:-}" ] && [ -n "$(key_service_account_id)" ]; then
  BRAINTRUST_OWNER_SERVICE_TOKEN=$BRAINTRUST_API_KEY
fi
if grep -q 'variable "owner_service_token"' "$src/main.tf" &&
  [ -z "${TF_VAR_owner_service_token:-}${BRAINTRUST_OWNER_SERVICE_TOKEN:-}" ]; then
  echo "$name creates service tokens; use an org-owner service token as BRAINTRUST_API_KEY or set BRAINTRUST_OWNER_SERVICE_TOKEN" >&2
  exit 1
fi

# Undeclared TF_VAR_* values are ignored, so set every example's variables.
: "${TF_VAR_my_email:=$email}"
: "${TF_VAR_admin_emails:=[\"$email\"]}"
: "${TF_VAR_auditor_emails:=[\"$email\"]}"
: "${TF_VAR_contractor_emails:=[\"$email\"]}"
[ -n "${TF_VAR_teams:-}" ] ||
  TF_VAR_teams="{ Search = { leads = [\"$email\"], engineers = [\"$email\"] }, Chat = { leads = [\"$email\"], engineers = [\"$email\"] } }"
: "${TF_VAR_openai_api_key:=sk-example-not-a-real-key}"
: "${TF_VAR_alert_webhook_url:=https://example.com/braintrust-alerts}"
: "${TF_VAR_owner_service_token:=${BRAINTRUST_OWNER_SERVICE_TOKEN:-}}"
export TF_VAR_my_email TF_VAR_admin_emails TF_VAR_auditor_emails TF_VAR_contractor_emails TF_VAR_teams \
  TF_VAR_openai_api_key TF_VAR_alert_webhook_url TF_VAR_owner_service_token

tf() { terraform -chdir="$state" "$@" -input=false; }

destroy() {
  echo "Destroying $name"
  if ! tf destroy -auto-approve; then
    echo "destroy failed; state kept in $state" >&2
    return 1
  fi
  rm -rf "$state"
}

# Remove the provider build, destroy after apply-then-destroy even if apply
# failed, and drop state dirs that never recorded any resources.
bin=
started=
cleanup() {
  status=$?
  trap - EXIT INT TERM
  if [ -n "$started" ] && [ "$mode" = apply-then-destroy ] && [ -f "$state/terraform.tfstate" ]; then
    destroy || status=1
  fi
  if [ -d "$state" ] && [ ! -f "$state/terraform.tfstate" ]; then
    rm -rf "$state"
  fi
  rmdir .example-state 2>/dev/null || true
  [ -z "$bin" ] || rm -rf "$bin"
  exit "$status"
}
trap cleanup EXIT
trap 'exit 130' INT TERM

# The state dir doubles as the Terraform working dir; refresh its config.
mkdir -p "$state"
echo "$BRAINTRUST_API_URL" >"$state/api_url"
rm -f "$state"/*.tf
cp "$src"/*.tf "$state/"

bin=$(mktemp -d "${TMPDIR:-/tmp}/bt-example-bin.XXXXXX")
go build -o "$bin/terraform-provider-braintrust" .
cat >"$bin/terraform.rc" <<EOF
provider_installation {
  dev_overrides {
    "braintrustdata/braintrust" = "$bin"
  }
  direct {}
}
EOF
export TF_CLI_CONFIG_FILE="$bin/terraform.rc" TF_IN_AUTOMATION=1 CHECKPOINT_DISABLE=1

started=1
echo "Running $name ($mode) against $BRAINTRUST_API_URL as $email"
case "$mode" in
plan)
  tf plan
  ;;
destroy)
  destroy
  ;;
apply | apply-then-destroy)
  if ! tf apply -auto-approve; then
    [ "$mode" = apply ] && echo "apply failed; clean up with: make run-example MODE=destroy examples/$name" >&2
    exit 1
  fi
  echo "Checking that a second plan is empty"
  plan_status=0
  tf plan -detailed-exitcode || plan_status=$?
  if [ "$mode" = apply ]; then
    echo "Resources kept. Clean up with: make run-example MODE=destroy examples/$name"
  fi
  case "$plan_status" in
  0) ;;
  2) echo "second plan was not empty; $name does not converge after one apply" >&2; exit 1 ;;
  *) exit "$plan_status" ;;
  esac
  ;;
esac
