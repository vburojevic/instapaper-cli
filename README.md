# instapaper-cli (ip)

A dependency-free (stdlib-only) Go CLI for the Instapaper **Full API**.

Features:
- OAuth 1.0a signed requests (HMAC-SHA1)
- xAuth login (`/api/1/oauth/access_token`)
- Bookmarks: add, list, archive/unarchive, star/unstar, move, delete, get_text
- Folders: list, add, delete, set_order
- Highlights: list, add, delete

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
./ip list --folder archive --json
./ip list --ndjson
./ip list --have "123:0.5:1700000000" --highlights "123,456"
./ip list --plain --output bookmarks.txt
./ip list --folder "My Folder"  # resolves folder title
```

By default, `list` returns all bookmarks (no limit) unless `defaults.list_limit` is set in config.

## Mutations

```bash
./ip archive 123456
./ip unarchive 123456
./ip star 123456
./ip unstar 123456
./ip move 123456 --folder "Work"

# Permanent delete (requires explicit flag)
./ip delete 123456 --yes-really-delete
```

## Get text view HTML

```bash
./ip text 123456 --out article.html
./ip text 123456 --out article.html --open
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

## AI agent usage

This CLI is optimized for agent workflows. Use structured output and exit codes for reliable parsing.

- `--json` for single objects (auth status, config, or single operations).
- `--ndjson` (or `--jsonl`) for streaming lists; each line is a full JSON object.
- `--plain` for stable, line-oriented text output.

Examples:

```bash
./ip --json auth status
./ip --json config show
./ip list --ndjson --limit 0
./ip list --plain --output bookmarks.txt
```

## Environment variables

- `INSTAPAPER_CONSUMER_KEY`
- `INSTAPAPER_CONSUMER_SECRET`
- `INSTAPAPER_API_BASE` (optional; defaults to `https://www.instapaper.com`)

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
