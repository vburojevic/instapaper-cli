# Release checklist

1. Update `CHANGELOG.md` (move items from Unreleased to the new version).
2. Ensure tests pass: `go test ./...`.
3. Tag the release: `git tag vX.Y.Z`.
4. Push commits and tag: `git push origin main --tags`.
5. Verify GitHub Actions release job succeeded (GoReleaser + Homebrew tap update).
6. Validate install: `brew update && brew upgrade instapaper-cli` or `brew install instapaper-cli`.
