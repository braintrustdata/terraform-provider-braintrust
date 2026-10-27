# Helpers for scripts that look up test fixtures from BRAINTRUST_API_KEY.
# Source after setting BRAINTRUST_API_KEY and BRAINTRUST_API_URL.

api() {
  curl -fsS --max-time 30 -H "Authorization: Bearer $BRAINTRUST_API_KEY" "$BRAINTRUST_API_URL$1"
}

# Prints the ID of the key's only org; fails with "<count> orgs; $1" if the
# key can access several.
key_org_id() {
  api '/v1/organization?limit=100' |
    jq -er --arg hint "$1" 'if (.objects | length) == 1 then .objects[0].id else error("key can access \(.objects | length) orgs; \($hint)") end'
}

# Prints the org's first human member as JSON. /v1/user lists human members
# only, so this works for service-token keys too.
key_first_user() {
  api '/v1/user?limit=1' | jq -ec '.objects[0]'
}

# Prints the key's own user ID if it is a service token, or nothing otherwise.
key_service_account_id() {
  api /api/self/me | jq -r 'if (.email | startswith("bt::sp::")) then .id else "" end'
}
