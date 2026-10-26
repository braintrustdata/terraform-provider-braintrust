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

api() {
  curl -fsS --max-time 30 -H "Authorization: Bearer $BRAINTRUST_API_KEY" "$BRAINTRUST_API_URL$1"
}

if [ -z "${BRAINTRUST_TEST_ORG_ID:-}" ]; then
  BRAINTRUST_TEST_ORG_ID=$(api '/v1/organization?limit=100' |
    jq -er 'if (.objects | length) == 1 then .objects[0].id else error("key can access \(.objects | length) orgs; set BRAINTRUST_TEST_ORG_ID") end')
fi

if [ -z "${BRAINTRUST_TEST_USER_ID:-}" ] || [ -z "${BRAINTRUST_TEST_USER_EMAIL:-}" ]; then
  # /v1/user lists human members only, so this works for service-token keys too.
  user=$(api '/v1/user?limit=1' | jq -ec '.objects[0]')
  : "${BRAINTRUST_TEST_USER_ID:=$(echo "$user" | jq -er .id)}"
  : "${BRAINTRUST_TEST_USER_EMAIL:=$(echo "$user" | jq -er .email)}"
fi

# A service-token key's own user is its service account.
if [ -z "${BRAINTRUST_SERVICE_ACCOUNT_ID:-}" ]; then
  BRAINTRUST_SERVICE_ACCOUNT_ID=$(api /api/self/me | jq -r 'if (.email | startswith("bt::sp::")) then .id else "" end')
fi

export BRAINTRUST_API_URL BRAINTRUST_TEST_ORG_ID BRAINTRUST_TEST_USER_ID BRAINTRUST_TEST_USER_EMAIL BRAINTRUST_SERVICE_ACCOUNT_ID
export BRAINTRUST_ORG_ID="${BRAINTRUST_ORG_ID:-$BRAINTRUST_TEST_ORG_ID}"
export TF_ACC=1

echo "Running acceptance tests against $BRAINTRUST_API_URL (org $BRAINTRUST_TEST_ORG_ID)"
exec go test -v -mod=readonly -count=1 -p 4 -parallel 4 -timeout 1200s ./internal/provider/ "$@"
