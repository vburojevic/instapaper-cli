# Release checklist

1. Update `CHANGELOG.md` (move items from Unreleased to the new version).
2. Ensure tests pass: `go test ./...`.
3. Tag and push via script (recommended): `scripts/release.sh vX.Y.Z`.
   - Or manually: `git tag -a vX.Y.Z -m "vX.Y.Z"` then `git push origin main && git push origin vX.Y.Z`.
5. Verify GitHub Actions release job succeeded (GoReleaser + Homebrew tap update).
6. If the tap was not updated (missing token), run `scripts/update-brew-tap.sh vX.Y.Z`.
7. Validate install: `brew update && brew upgrade instapaper-cli` or `brew install instapaper-cli`.

Notes:
- GoReleaser runs from `.github/workflows/release.yml` on tag push.
- Set `HOMEBREW_TAP_GITHUB_TOKEN` in GitHub Actions to enable auto tap updates.
