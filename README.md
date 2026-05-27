# Gmail Organizer

Local-first Gmail organizer for reviewing, categorizing, deleting, and preparing unsubscribe actions for high-volume inbox cleanup.

## Architecture

- Go backend for low memory overhead, secure local API boundaries, Gmail OAuth integration, and batched metadata-only email reads.
- React + Vite dashboard for category lanes, inbox review, AI categorization, and bulk actions.
- Secrets stay outside the repo. The app references `GOOGLE_CLIENT_SECRET_FILE` and `OPENAI_API_KEY_FILE` paths and never returns secret values from the API.

## Current Status

This is the initial working slice. It includes:

- Gmail OAuth URL and callback endpoints.
- Gmail metadata fetch using `gmail.modify` scope.
- Heuristic categorization and optional OpenAI Responses API categorization.
- Dashboard lanes by category.
- Explicit AI toggle for scan/monitor jobs with bounded backend AI classification chunks.
- Manual category correction for selected emails.
- Sender rules from manual category corrections for future monitoring/scanning.
- Persisted review coverage metrics across scans and manual moves.
- Bulk trash, mark-read, and unsubscribe-preparation actions.
- Two-step confirmation for destructive trash and one-click unsubscribe actions.
- One-click unsubscribe execution for Gmail messages that advertise `List-Unsubscribe-Post: List-Unsubscribe=One-Click`.
- Paged mailbox scanning for larger cleanup passes without loading the full mailbox into memory.
- Demo data fallback when Gmail is not authenticated.

## Setup

From the repo root:

```powershell
go mod tidy
cd web
npm install
npm run build
cd ..
go run ./cmd/server
```

Default secret discovery looks up parent directories for:

- `client_secret*.json`
- `openai_key.txt`

You can override paths without exposing values:

```powershell
$env:GOOGLE_CLIENT_SECRET_FILE="<absolute path to client_secret*.json>"
$env:OPENAI_API_KEY_FILE="<absolute path to openai_key.txt>"
```

Open `http://127.0.0.1:8787`.

Optional monitoring settings:

```powershell
$env:GMAIL_ORGANIZER_MONITOR_INTERVAL_SECONDS="60"
$env:GMAIL_ORGANIZER_MONITOR_CACHE_LIMIT="500"
$env:GMAIL_ORGANIZER_SCAN_CACHE_LIMIT="1000"
```

## Screenshot

![Dashboard screenshot](docs/screenshot.png)

## Security Notes

- The backend binds to `127.0.0.1` by default.
- Email listing fetches metadata headers and snippets, not full message bodies.
- Bulk delete uses Gmail trash, not immediate permanent deletion.
- Trash and one-click unsubscribe actions return a preview first and require an explicit confirm request before execution.
- Unsubscribe actions execute only standards-based HTTPS one-click requests; ordinary HTTPS and `mailto:` unsubscribe targets are prepared as review links.
- API responses include secret file paths and existence status only, never secret contents.
- Background monitoring keeps a bounded in-memory cache and uses metadata/snippets rather than full message bodies.
- Mailbox scans fetch Gmail metadata in pages, persist classifications after each batch, and keep only a bounded recent cache in memory.
- Review coverage stats are derived from local classification state and do not require reloading message bodies.
- Sender rules are stored locally and apply to future emails after classifier output but before per-message overrides.
- AI scan/monitor classification is opt-in and chunked so prompts stay bounded.

## Verification

```powershell
go test ./...
cd web
npm run build
npm audit --json
```

The current dashboard screenshot was captured from a running local backend at `http://127.0.0.1:8787`.
