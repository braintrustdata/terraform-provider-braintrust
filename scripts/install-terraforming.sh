#!/usr/bin/env sh

set -eu

REPO="braintrustdata/terraform-provider-braintrust"

detect_os() {
  os="$(uname -s | tr '[:upper:]' '[:lower:]')"
  case "$os" in
    linux*) echo "linux" ;;
    darwin*) echo "darwin" ;;
    msys*|mingw*|cygwin*) echo "windows" ;;
    *)
      echo "unsupported OS: $os" >&2
      exit 1
      ;;
  esac
}

detect_arch() {
  arch="$(uname -m)"
  case "$arch" in
    x86_64|amd64) echo "amd64" ;;
    arm64|aarch64) echo "arm64" ;;
    *)
      echo "unsupported architecture: $arch" >&2
      exit 1
      ;;
  esac
}

get_latest_version() {
  if [ -n "${VERSION:-}" ]; then
    echo "$VERSION"
    return
  fi

  if use_gh; then
    gh release view --repo "$REPO" --json tagName --jq .tagName
    return
  fi

  curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" |
    sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' |
    head -n1
}

# Prefer an authenticated gh CLI so private/internal releases can be installed.
use_gh() {
  command -v gh >/dev/null 2>&1 && gh auth status >/dev/null 2>&1
}

main() {
  if ! use_gh && ! command -v curl >/dev/null 2>&1; then
    echo "gh or curl is required to install braintrust-terraforming" >&2
    exit 1
  fi
  if ! command -v tar >/dev/null 2>&1; then
    echo "tar is required to install braintrust-terraforming" >&2
    exit 1
  fi

  os="$(detect_os)"
  arch="$(detect_arch)"
  version="$(get_latest_version)"
  if [ -z "$version" ]; then
    echo "could not determine latest release version" >&2
    exit 1
  fi

  clean_version="${version#v}"
  artifact="braintrust-terraforming_${clean_version}_${os}_${arch}.tar.gz"
  url="https://github.com/$REPO/releases/download/${version}/${artifact}"

  tmpdir="$(mktemp -d)"
  trap 'rm -rf "$tmpdir"' EXIT INT TERM

  echo "Downloading ${url}"
  if use_gh; then
    gh release download "$version" --repo "$REPO" --pattern "$artifact" --dir "$tmpdir"
  else
    curl -fL "$url" -o "$tmpdir/$artifact"
  fi
  tar -xzf "$tmpdir/$artifact" -C "$tmpdir"

  target_dir="${INSTALL_DIR:-/usr/local/bin}"
  if [ ! -w "$target_dir" ]; then
    target_dir="${HOME}/.local/bin"
    mkdir -p "$target_dir"
  fi

  install -m 0755 "$tmpdir/braintrust-terraforming" "$target_dir/braintrust-terraforming"
  echo "Installed to $target_dir/braintrust-terraforming"
  echo "Run: braintrust-terraforming version"
}

main "$@"
