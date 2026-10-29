#!/usr/bin/env sh

# Runs the acceptance tests against the API at BRAINTRUST_API_URL (default
# https://api.braintrust.dev). The test org and user default to the key's only
# org and its first human member, and the service account defaults to the key's
# own when the key is a service token; set BRAINTRUST_TEST_ORG_ID,
# BRAINTRUST_TEST_USER_ID, BRAINTRUST_TEST_USER_EMAIL, or
# BRAINTRUST_SERVICE_ACCOUNT_ID to override. Extra arguments go to `go test`,
# e.g. `scripts/test-acceptance.sh -run TestAccProject`.

set -eu

cd "$(dirname "$0")/.."

: "${BRAINTRUST_API_KEY:?BRAINTRUST_API_KEY must be set}"
: "${BRAINTRUST_API_URL:=https://api.braintrust.dev}"

. scripts/braintrust-key.sh

if [ -z "${BRAINTRUST_TEST_ORG_ID:-}" ]; then
  BRAINTRUST_TEST_ORG_ID=$(key_org_id "set BRAINTRUST_TEST_ORG_ID")
fi

if [ -z "${BRAINTRUST_TEST_USER_ID:-}" ] || [ -z "${BRAINTRUST_TEST_USER_EMAIL:-}" ]; then
  user=$(key_first_user)
  : "${BRAINTRUST_TEST_USER_ID:=$(echo "$user" | jq -er .id)}"
  : "${BRAINTRUST_TEST_USER_EMAIL:=$(echo "$user" | jq -er .email)}"
fi

# A service-token key's own user is its service account.
if [ -z "${BRAINTRUST_SERVICE_ACCOUNT_ID:-}" ]; then
  BRAINTRUST_SERVICE_ACCOUNT_ID=$(key_service_account_id)
fi

export BRAINTRUST_API_URL BRAINTRUST_TEST_ORG_ID BRAINTRUST_TEST_USER_ID BRAINTRUST_TEST_USER_EMAIL BRAINTRUST_SERVICE_ACCOUNT_ID
export BRAINTRUST_ORG_ID="${BRAINTRUST_ORG_ID:-$BRAINTRUST_TEST_ORG_ID}"
export TF_ACC=1

echo "Running acceptance tests against $BRAINTRUST_API_URL (org $BRAINTRUST_TEST_ORG_ID)"
exec go test -v -mod=readonly -count=1 -p 4 -parallel 4 -timeout 1200s ./internal/provider/ "$@"
