#!/usr/bin/env bash
set -euo pipefail

tag="${1:-}"
if [[ -z "$tag" ]]; then
  echo "usage: scripts/update-brew-tap.sh vX.Y.Z [tap-repo]" >&2
  exit 1
fi

tap_repo="${2:-${HOMEBREW_TAP_REPO:-}}"
if [[ -z "$tap_repo" ]]; then
  tap_repo="$(gh repo list --limit 200 --json nameWithOwner,name -q '.[] | select(.name | test("tap|homebrew"; "i")) | .nameWithOwner' | head -n1)"
fi
if [[ -z "$tap_repo" ]]; then
  echo "tap repo not found; pass it as arg or set HOMEBREW_TAP_REPO" >&2
  exit 1
fi

repo="$(gh repo view --json nameWithOwner -q .nameWithOwner)"
url="https://github.com/${repo}/archive/refs/tags/${tag}.tar.gz"

tmp_dir="$(mktemp -d)"
trap 'rm -rf "$tmp_dir"' EXIT

curl -sSL "$url" -o "$tmp_dir/src.tar.gz"
sha="$(shasum -a 256 "$tmp_dir/src.tar.gz" | awk '{print $1}')"

# Clone tap repo and locate formula.
gh repo clone "$tap_repo" "$tmp_dir/tap" >/dev/null 2>&1
formula_path="$(find "$tmp_dir/tap" -name 'instapaper-cli.rb' -print -quit)"
if [[ -z "$formula_path" ]]; then
  echo "instapaper-cli.rb not found in $tap_repo" >&2
  exit 1
fi

perl -0pi -e "s|url \".*\"|url \"$url\"|" "$formula_path"
perl -0pi -e "s|sha256 \".*\"|sha256 \"$sha\"|" "$formula_path"

git -C "$tmp_dir/tap" add "$formula_path"
git -C "$tmp_dir/tap" commit -m "instapaper-cli ${tag}" >/dev/null

git -C "$tmp_dir/tap" push >/dev/null

echo "Updated $formula_path in $tap_repo"
