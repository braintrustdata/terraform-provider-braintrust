#!/usr/bin/env sh

# Fails if any tool version pinned outside mise.toml drifts from mise.toml.

set -eu

cd "$(dirname "$0")/.."

status=0

# Prints the version of tool $1 from mise.toml.
mise_version() {
  sed -n "s/^$1 *= *\"\(.*\)\"/\1/p" mise.toml
}

# Fails unless every version in the remaining arguments equals $2.
expect() {
  what="$1"
  want="$2"
  shift 2
  if [ -z "$want" ]; then
    echo "mise.toml: missing $what" >&2
    status=1
    return
  fi
  if [ "$#" -eq 0 ]; then
    echo "$what: no pinned versions found; update $0" >&2
    status=1
  fi
  for got in "$@"; do
    if [ "$got" != "$want" ]; then
      echo "$what: found $got, but mise.toml pins $want" >&2
      status=1
    fi
  done
}

# Prints the "version:" input of each step using GitHub action $1.
action_versions() {
  awk -v action="$1" '
    $0 ~ "uses: " action "@" { in_step = 1; next }
    in_step && /^ *- / { in_step = 0 }
    in_step && /^ *version:/ { sub(/^ *version: *v?/, ""); print }
  ' .github/workflows/*.yml
}

expect "go (go.mod)" "$(mise_version go)" "$(sed -n 's/^go //p' go.mod)"

# shellcheck disable=SC2046
expect "terraform (.terraform-version, workflows)" "$(mise_version terraform)" \
  $(cat .terraform-version) \
  $(grep -ho 'terraform[_ /][0-9][0-9.]*[0-9]' .github/workflows/*.yml | sed 's/^terraform[_ /]//')

# shellcheck disable=SC2046
expect "golangci-lint (workflows)" "$(mise_version golangci-lint)" \
  $(action_versions golangci/golangci-lint-action)

# shellcheck disable=SC2046
expect "goreleaser (workflows)" "$(mise_version goreleaser)" \
  $(action_versions goreleaser/goreleaser-action)

if [ "$status" -eq 0 ]; then
  echo "Tool versions match mise.toml."
fi
exit "$status"
