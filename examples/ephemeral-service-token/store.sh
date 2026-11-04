#!/usr/bin/env sh
# Demo-only plaintext storage: first line is the token ID, second is the key.
# Never print the key, place it in command arguments, or overwrite an entry.
set -eu
umask 077

: "${SECRET_FILE:?SECRET_FILE must be set}"
: "${TOKEN_ID_FILE:?TOKEN_ID_FILE must be set}"

directory=${SECRET_FILE%/*}
if [ -L "$directory" ]; then
  echo "Refusing a symlinked secret directory" >&2
  exit 1
fi
if [ -L "$SECRET_FILE" ] || [ -L "$TOKEN_ID_FILE" ]; then
  echo "Refusing a symlinked secret entry" >&2
  exit 1
fi

case "${1:?usage: store.sh create|destroy}" in
destroy)
  : "${BRAINTRUST_API_KEY:?BRAINTRUST_API_KEY must be set to revoke the token}"
  : "${API_URL:?API_URL must be set}"
  # The non-secret receipt survives loss of the key or failed delivery.
  # Fall back to the old demo's entry when adopting an existing store.
  id_file=$TOKEN_ID_FILE
  [ -f "$id_file" ] || id_file=$SECRET_FILE
  if [ ! -f "$id_file" ]; then
    echo "Token receipt missing; reconcile this owner's tokens in Braintrust before removing its Terraform state" >&2
    exit 1
  fi
  IFS= read -r token_id < "$id_file"
  case "$token_id" in
    "" | *[!a-fA-F0-9-]*) echo "Invalid stored token ID; refusing cleanup" >&2; exit 1 ;;
  esac
  [ "${#token_id}" -eq 36 ] || { echo "Invalid stored token ID; refusing cleanup" >&2; exit 1; }
  # Keep credentials out of process arguments and response bodies off disk.
  response=$(printf 'Authorization: Bearer %s\n' "$BRAINTRUST_API_KEY" |
    curl -q --silent --show-error --max-time 30 --header @- \
      --request DELETE --write-out '\n%{http_code}' \
      "${API_URL%/}/v1/service_token/$token_id")
  # curl appends the status on the final line, after the response body.
  status=${response##*'
'}
  case "$status" in
    2?? | 404) ;;
    400 | 403)
      # Match the provider's missing-resource semantics, not arbitrary
      # permission errors. jq slurps the JSON body and appended status.
      if ! printf '%s' "$response" | jq -es '
        (.[0].error // .[0].message // "") | strings |
        ascii_downcase | contains("does not exist")' >/dev/null; then
        echo "Token revocation failed (HTTP $status); store retained for retry" >&2
        exit 1
      fi
      ;;
    *) echo "Token revocation failed (HTTP $status); store retained for retry" >&2; exit 1 ;;
  esac
  rm -f "$SECRET_FILE"
  rm -f "$TOKEN_ID_FILE"
  # Preserve unrelated or interrupted-delivery files.
  rmdir "$directory" 2>/dev/null || true
  exit 0
  ;;
create)
  # Applying the upgraded demo can take ownership of an existing demo entry.
  if [ -f "$SECRET_FILE" ] && [ -z "${TOKEN_ID:-}" ] && [ -z "${TOKEN_KEY:-}" ]; then
    if [ ! -f "$TOKEN_ID_FILE" ]; then
      IFS= read -r token_id < "$SECRET_FILE"
      (set -C; printf '%s\n' "$token_id" > "$TOKEN_ID_FILE")
    fi
    exit 0
  fi
  # A receipt without an entry means an earlier delivery failed after minting.
  if [ -z "${TOKEN_ID:-}" ] && [ -f "$TOKEN_ID_FILE" ]; then
    IFS= read -r token_id < "$TOKEN_ID_FILE"
    echo "Token $token_id was issued but its key was not stored, and the key cannot be recovered. Run destroy to revoke it, then apply again." >&2
    exit 1
  fi
  : "${TOKEN_ID:?TOKEN_ID must be set}"
  : "${TOKEN_KEY:?TOKEN_KEY must be set}"
  # Record the non-secret ID before filesystem operations that might fail.
  # Never overwrite an earlier receipt: reconcile it before retrying.
  (set -C; printf '%s\n' "$TOKEN_ID" > "$TOKEN_ID_FILE")
  ;;
*) echo "usage: store.sh create|destroy" >&2; exit 1 ;;
esac
mkdir -p "$directory"
chmod 700 "$directory"
if [ -e "$SECRET_FILE" ] || [ -L "$SECRET_FILE" ]; then
  echo "Secret entry already exists; refusing to overwrite it" >&2
  exit 1
fi

temporary=$(mktemp "$directory/.service-token.XXXXXX")
trap 'rm -f "$temporary"' EXIT
trap 'exit 1' HUP INT TERM
printf '%s\n%s\n' "$TOKEN_ID" "$TOKEN_KEY" > "$temporary"
chmod 600 "$temporary"
# Publishing a complete file with a hard link is atomic and does not replace
# an existing file if another writer won after the existence check above.
ln "$temporary" "$SECRET_FILE"
