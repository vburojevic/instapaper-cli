#!/usr/bin/env bash
set -euo pipefail

tag="${1:-}"
if [[ -z "$tag" ]]; then
  echo "usage: scripts/release.sh vX.Y.Z" >&2
  exit 1
fi
if [[ ! "$tag" =~ ^v[0-9] ]]; then
  echo "tag must start with v (example: v0.2.7)" >&2
  exit 1
fi
if ! git diff --quiet || ! git diff --cached --quiet; then
  echo "working tree has uncommitted changes" >&2
  exit 1
fi
if git rev-parse "$tag" >/dev/null 2>&1; then
  echo "tag $tag already exists" >&2
  exit 1
fi

git tag -a "$tag" -m "$tag"
# Push commits and tag to trigger the release workflow.
git push origin main
git push origin "$tag"

echo "Release tag pushed: $tag"
echo "GitHub Actions will run GoReleaser and update the Homebrew tap if a token is configured."
echo "If the tap was not updated, run: scripts/update-brew-tap.sh $tag"
