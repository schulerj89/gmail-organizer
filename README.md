# Gmail Organizer

Local-first Gmail organizer for reviewing, categorizing, deleting, and preparing unsubscribe actions for high-volume inbox cleanup.

## Architecture

- Go backend for low memory overhead, secure local API boundaries, Gmail OAuth integration, and batched metadata-only email reads.
- React + Vite cleanup workbench for category queues, email detail review, AI categorization, and bulk actions.
- Secrets stay outside the repo. The app references `GOOGLE_CLIENT_SECRET_FILE` and `OPENAI_API_KEY_FILE` paths and never returns secret values from the API.

## Current Status

This is the initial working slice. It includes:

- Gmail OAuth URL and callback endpoints.
- Gmail metadata fetch using `gmail.modify` scope.
- Heuristic categorization and optional OpenAI Responses API categorization.
- First-run guided dashboard tutorial with local browser storage for completed/skipped state and a restart control.
- Left-nav cleanup workbench with one contained email queue instead of long category lanes.
- Quick cleanup queues for unsubscribe-ready messages and suggested cleanup candidates.
- Sender cleanup queue that groups unsubscribe-ready messages by sender for faster bulk review.
- AI suggestions queue for accepting high-confidence category decisions in bulk or after inspection.
- Email detail dialog with sender, subject, snippet, category reason, unsubscribe availability, and single-message actions.
- Review decision controls in the email detail dialog for correcting category and applying future sender rules.
- Contextual bulk action bar that appears only after selecting emails.
- Modal preview and confirmation flow for unsubscribe and move-to-trash actions.
- Saved category totals and left-nav category loading for SQLite-backed review pages.
- User-friendly Gmail date search presets plus custom before/after calendar date controls.
- Explicit AI toggle for scan/monitor jobs with bounded backend AI classification chunks.
- Manual category correction for selected emails.
- Sender rules from manual category corrections for future monitoring/scanning.
- SQLite-backed persisted review coverage metrics across scans and manual moves.
- Reload stored category pages from prior scans for later review and cleanup.
- Bulk trash, mark-read, and unsubscribe-preparation actions.
- Batched Gmail mark-read updates for high-volume cleanup selections.
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

## Gmail API Permissions

The app requests this Gmail OAuth scope:

- `https://www.googleapis.com/auth/gmail.modify`

This is required because Gmail Organizer reads Gmail message metadata, marks messages as read, and moves selected messages to Gmail Trash. The app does **not** permanently delete messages with Gmail's delete endpoint; trash actions use Gmail Trash so messages remain recoverable in Gmail for the normal Trash retention window.

Google describes `gmail.modify` as read/write Gmail access except immediate permanent deletion that bypasses Trash. See Google's Gmail API scope reference: <https://developers.google.com/workspace/gmail/api/auth/scopes>.

## Google OAuth Setup

Create the Gmail OAuth client before starting the app:

1. Open Google Cloud Console and create or select a project.
2. Go to **APIs & Services > Library**, search for **Gmail API**, and enable it.
3. Go to **APIs & Services > OAuth consent screen**.
4. Choose **External** for a personal Gmail account, keep the app in **Testing**, and add your Gmail address under **Test users**.
5. On the consent screen data/scopes step, add the Gmail API scope `https://www.googleapis.com/auth/gmail.modify` if Google prompts you to list scopes.
6. Go to **APIs & Services > Credentials**.
7. Create an **OAuth client ID**. For this local callback flow, use **Web application**.
8. Add the exact authorized redirect URI that matches how you run the app:
   - Default local run: `http://127.0.0.1:8787/api/auth/google/callback`
   - Port 8080 callback: `http://localhost:8080/oauth2callback`
9. Download the OAuth client JSON and save it somewhere outside the repo, or as a local ignored `client_secret*.json` file.
10. Point the app at it:

```powershell
$env:GOOGLE_CLIENT_SECRET_FILE="<absolute path to client_secret*.json>"
```

If you change scopes, redirect URLs, or OAuth client files after already authorizing, delete `data/gmail_token.json` and click **Authorize** again in the app so Google issues a fresh token.

For OAuth client background and redirect URI rules, see Google's OAuth setup documentation: <https://support.google.com/googleapi/answer/6158849>.

Open `http://127.0.0.1:8787`.

If your Google OAuth client is registered with `http://localhost:8080/oauth2callback`, start the app with the matching local callback:

```powershell
$env:GMAIL_ORGANIZER_PORT="8080"
$env:GMAIL_ORGANIZER_OAUTH_REDIRECT_URL="http://localhost:8080/oauth2callback"
go run ./cmd/server
```

Then open `http://localhost:8080`. While the OAuth consent screen is in testing mode, add the Gmail account you are signing in with under the Google Cloud OAuth consent screen test users list.

Optional monitoring settings:

```powershell
$env:GMAIL_ORGANIZER_MONITOR_INTERVAL_SECONDS="60"
$env:GMAIL_ORGANIZER_MONITOR_CACHE_LIMIT="500"
$env:GMAIL_ORGANIZER_SCAN_CACHE_LIMIT="1000"
```

Optional OpenAI safety settings:

```powershell
$env:OPENAI_MAX_OUTPUT_TOKENS="2000"
$env:OPENAI_MAX_RETRIES="3"
$env:OPENAI_REQUEST_DELAY_MS="1200"
$env:OPENAI_CLASSIFY_CHUNK_SIZE="25"
$env:OPENAI_TIMEOUT_SECONDS="45"
```

## Screenshots

Dashboard overview:

![Dashboard overview](docs/screenshot.png)

Dark mode dashboard:

![Dark mode dashboard](docs/screenshot-dark.png)

Guided tutorial overlay:

![Guided tutorial overlay](docs/screenshot-tutorial.png)

Cleanup preview and confirmation:

![Cleanup preview](docs/screenshot-cleanup-preview.png)

Review decision modal:

![Review decision modal](docs/screenshot-review-decision.png)

AI suggestions queue:

![AI suggestions queue](docs/screenshot-ai-suggestions.png)

## Security Notes

- The backend binds to `127.0.0.1` by default.
- Mutating API requests with non-local browser origins are blocked.
- Email listing fetches metadata headers and snippets, not full message bodies.
- Bulk delete uses Gmail trash, not immediate permanent deletion.
- Trash and one-click unsubscribe actions return a preview first and require a short-lived server-issued confirmation token before execution.
- Unsubscribe actions execute only standards-based HTTPS one-click requests; ordinary HTTPS and `mailto:` unsubscribe targets are prepared as review links.
- API responses include secret file paths and existence status only, never secret contents.
- Background monitoring keeps a bounded in-memory cache and uses metadata/snippets rather than full message bodies.
- Mailbox scans fetch Gmail metadata in pages, persist classifications after each batch, and keep only a bounded recent cache in memory.
- Review coverage stats are derived from local classification state and do not require reloading message bodies.
- Stored category pages preserve minimal metadata needed for later review/actions without keeping the full scan in memory.
- Review state, sender rules, and action audit entries are stored in `data/review_state.db`; older JSON state files are imported into SQLite on first startup.
- Successfully trashed messages are removed from local review state after the action is audited.
- Sender rules are stored locally and apply to future emails after classifier output but before per-message overrides.
- AI scan/monitor classification is opt-in and chunked so prompts stay bounded.
- OpenAI classification uses configurable chunk size, output-token cap, request pacing, timeout, and retry settings for rate-limit-aware scanning.
- Mark-read actions use Gmail batch modify calls of up to 1000 message IDs per request.
- Action audit reads allow bounded large entries so 1000-message cleanup results remain reviewable.

## Verification

```powershell
go test ./...
cd web
npm run build
npm audit --json
```

The current screenshots were captured from a running local backend at `http://127.0.0.1:8080`.
