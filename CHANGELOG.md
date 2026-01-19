# Changelog

All notable changes to this project will be documented in this file.

## [Unreleased]

## [0.2.5] - 2026-01-19
- Allow flags after positional args for flag-based commands (add/list/export/progress/etc).
- Add tests for folder position floats and flag reordering.

## [0.2.4] - 2026-01-19
- Fix folder position parsing (float positions) and allow flags after bookmark id for `move`.

## [0.2.3] - 2026-01-19
- Skip Homebrew upload when tap token is missing.

## [0.2.2] - 2026-01-19
- Fix GoReleaser Homebrew config for v2.

## [0.2.1] - 2026-01-19
- Fix GoReleaser configuration for v2.

## [0.2.0] - 2026-01-19
- Default output to NDJSON and add agent-focused help and docs.
- Add list/export cursor support, fields selection, and NDJSON output tests.
- Add import/export commands and JSON schema output.
- Add health/verify commands, config get/set/unset, and structured stderr errors.
- Add CI (tests + lint), Makefile tasks, troubleshooting docs, and release checklist.
- Add list/export bounds (`--since`, `--until`, `--updated-since`), cursor-dir, and max-pages.
- Add bulk mutation flags (`--ids`, `--stdin`, `--batch`) and progress events for import.
- Add `--select` client-side filter, `--verbose`, and paged export output (`--output-dir`).

## [0.1.0] - 2026-01-19
- Initial public release.
