# instapaper-cli (ip)

A dependency-free (stdlib-only) Go CLI for the Instapaper **Full API**.

Features:
- OAuth 1.0a signed requests (HMAC-SHA1)
- xAuth login (`/api/1/oauth/access_token`)
- Bookmarks: add, list, export/import, archive/unarchive, star/unstar, move, delete, get_text
- Folders: list, add, delete, set_order
- Highlights: list, add, delete
- Health/verify checks, JSON schema output
- NDJSON/JSON/plain output, structured stderr (`--stderr-json`), retries, dry-run, idempotent mode
- Incremental sync (cursor files or bounds), bulk operations, and progress events
- Client-side filtering (`--select`), verbose summaries, and paged exports

## Install

```bash
go install github.com/vburojevic/instapaper-cli/cmd/ip@latest
ip version
```

Homebrew (via tap):

```bash
brew tap vburojevic/tap
brew install instapaper-cli
```

Or build from source:

```bash
go build ./cmd/ip
./ip version
```

Release process: see `docs/release-checklist.md`.

## Quickstart

```bash
export INSTAPAPER_CONSUMER_KEY="..."
export INSTAPAPER_CONSUMER_SECRET="..."
printf '%s' "your-password" | ./ip auth login --username "you@example.com" --password-stdin
./ip list --ndjson --limit 10
```

## Setup

You need an Instapaper API consumer key/secret.

Set them as env vars (recommended):

```bash
export INSTAPAPER_CONSUMER_KEY="..."
export INSTAPAPER_CONSUMER_SECRET="..."
```

Or pass them to `auth login`.

## Authenticate (xAuth)

```bash
# Interactive (password may echo depending on your OS; prefer --password-stdin)
./ip auth login --username "you@example.com" --password-stdin

# Example with stdin password:
printf '%s' "your-password" | ./ip auth login --username "you@example.com" --password-stdin

# Non-interactive (disable prompts):
printf '%s' "your-password" | ./ip auth login --username "you@example.com" --password-stdin --no-input
```

This stores only the OAuth access token + secret in your user config directory.
It **does not** store your Instapaper password.

Config location:

```bash
./ip config path
```

## Configuration

```bash
./ip config show
./ip config get defaults.format
./ip config set defaults.list_limit 100
./ip config unset defaults.resolve_final_url
```

## Add a URL

```bash
./ip add https://example.com/article \
  --title "Example" \
  --tags "go,readlater" \
  --folder unread
```

Add from stdin:

```bash
cat urls.txt | ./ip add -
```

## List

```bash
./ip list --folder unread --limit 25
./ip list --folder archive --format json
./ip list --folder archive --format table
./ip list --folder archive --json
./ip list --ndjson
./ip list --have "123:0.5:1700000000" --highlights "123,456"
./ip list --fields "bookmark_id,title,url" --ndjson
./ip list --cursor ~/.config/ip/cursor.json
./ip list --cursor-dir ~/.config/ip/cursors
./ip list --since bookmark_id:12345
./ip list --until time:2025-01-01T00:00:00Z
./ip list --updated-since 2025-01-01T00:00:00Z
./ip list --limit 0 --max-pages 50
./ip list --select "starred=1,tag~news"
./ip list --plain --output bookmarks.txt
./ip list --folder "My Folder"  # resolves folder title
```

By default, `list` returns all bookmarks (no limit) unless `defaults.list_limit` is set in config.

Bounds format for `--since/--until`:
- `bookmark_id:<id>` (default when no prefix is supplied)
- `time:<rfc3339|unix>`
- `progress_timestamp:<rfc3339|unix>`

Select format for `--select`:
- Comma-separated filters: `<field><op><value>`
- Operators: `=`, `!=`, `~` (contains, case-insensitive)
- Fields: `bookmark_id`, `time`, `progress`, `progress_timestamp`, `starred`, `title`, `url`, `description`, `tags`

## Output formats

- `--ndjson` (default): one JSON object per line (stream-friendly).
- `--json`: a single JSON array/object.
- `--plain`: stable, tab-delimited text (for pipes).
- `--format table`: human table (avoid for parsing).

Use `--output <file>` to write results to a file. Use `-` for stdout.
Use `--output-dir <dir>` on `export` to write each page as its own NDJSON file.
Use `--verbose` to emit summary counts to stderr (keeps stdout clean).

## Mutations

```bash
./ip archive 123456
./ip unarchive 123456
./ip star 123456
./ip unstar 123456
./ip move --folder "Work" 123456

# Permanent delete (requires explicit flag)
./ip delete 123456 --yes-really-delete
./ip delete 123456 --confirm 123456

# Bulk mutations
./ip archive --ids 1,2,3
printf "10\n11\n12\n" | ./ip unarchive --stdin
./ip delete --ids 5,6 --yes-really-delete
./ip archive --ids 1,2,3 --batch 2
```

Dry-run and idempotent modes:

```bash
./ip --dry-run archive 123456
./ip --idempotent highlights add 123456 --text "Some quote"
```

## Get text view HTML

```bash
./ip text 123456 --out article.html
./ip text 123456 --out article.html --open
printf "1\n2\n3\n" | ./ip text --stdin --out ./articles
```

## Update read progress

```bash
./ip progress 123456 --progress 0.5 --timestamp 1700000000
```

## Folders

```bash
./ip folders list
./ip folders add "New Folder"
./ip folders delete "New Folder" --yes

# Reorder folders: folder_id:position pairs (must include all folders)
./ip folders order "100:1,200:2,300:3"
```

## Highlights

```bash
./ip highlights list 123456
./ip highlights add 123456 --text "Some quote" --position 0
./ip highlights delete 98765
```

## Export & import

```bash
# Export all bookmarks (NDJSON by default)
./ip export --cursor ~/.config/ip/cursor.json
./ip export --cursor-dir ~/.config/ip/cursors
./ip export --since time:2025-01-01T00:00:00Z

# Export with specific fields
./ip export --fields "bookmark_id,title,url" --ndjson

# Export into a directory (paged NDJSON files)
./ip export --output-dir ./exports --cursor-dir ~/.config/ip/cursors

# Note: --output-dir requires NDJSON output (default)

# Import from plain text (one URL per line)
./ip import --input urls.txt --input-format plain

# Import from NDJSON
./ip import --input bookmarks.ndjson --input-format ndjson

# Import with progress events on stderr
./ip import --input bookmarks.ndjson --input-format ndjson --progress-json
```

## Progress events (NDJSON)

Use `--progress-json` to emit progress lines to stderr for long operations:

```bash
./ip import --input bookmarks.ndjson --input-format ndjson --progress-json
```

Write output to a file:

```bash
./ip list --format json --output bookmarks.json
```

## Health & verify

```bash
./ip health
./ip verify
```

## Schemas

```bash
./ip schema bookmarks
./ip schema auth
```

## AI agent usage

This CLI is optimized for agent workflows. Default output is NDJSON; use structured output and exit codes for reliable parsing.

- `--json` for single objects (auth status, config, or single operations).
- `--ndjson` (or `--jsonl`) for streaming lists; each line is a full JSON object.
- `--plain` for stable, line-oriented text output.
- `--stderr-json` for structured errors and hints on stderr.
- `--output` to write results to a file (use `-` for stdout).
- Run `ip help ai` for agent-focused tips.
- Use `--since/--until` or `--updated-since` for deterministic incremental pulls.
- Use `--cursor-dir` for auto cursor files per folder/tag.
- Use `--ids` or `--stdin` for bulk mutations; `--progress-json` for progress events.
- Use `--select` for client-side filtering when the API doesn't support it.

Examples:

```bash
./ip --json auth status
./ip --json config show
./ip list --ndjson --limit 0
./ip list --plain --output bookmarks.txt
```

## Help

```bash
./ip --help
./ip help list
./ip help ai
```

## Environment variables

- `INSTAPAPER_CONSUMER_KEY`
- `INSTAPAPER_CONSUMER_SECRET`
- `INSTAPAPER_API_BASE` (optional; defaults to `https://www.instapaper.com`)
- `INSTAPAPER_TIMEOUT` (optional; Go duration like `10s`, `1m`)

## Troubleshooting

- Auth errors: run `./ip auth status` or `./ip --json auth status` to verify tokens.
- Rate limits: error code `1040` means retry later; consider backing off.
- Config issues: `./ip config path` to locate your config; `./ip --json config show` to inspect values.
- Network problems: try `./ip --debug list --limit 1` to see request timing and status codes.

## Exit codes

- `0` success
- `1` generic failure
- `2` invalid usage
- `10` rate limited
- `11` premium required
- `12` application suspended
- `13` invalid request
- `14` server error

## Notes

- Instapaper's API Terms of Use prohibit storing user passwords. This CLI only stores OAuth tokens.
- For Windows users, `--password-stdin` is strongly recommended.

## API reference

```
https://www.instapaper.com/api
```
