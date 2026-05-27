# Context

## Goal

Build a secure, memory-efficient Gmail AI organizer that categorizes emails, monitors incoming mail, and supports mass delete/unsubscribe workflows from a modern dashboard.

## Current Implementation

- Backend: Go local HTTP API in `cmd/server` and `internal`.
- Frontend: React/Vite app in `web`.
- Dashboard: first-run tutorial overlay with local browser completion/skipped state, restart tutorial control, category lanes, per-lane stored totals/load controls, user-friendly Gmail date search presets with custom before/after calendar dates, max controls, manual refresh, scan, monitoring toggle, AI categorize, manual category moves, bulk read, unsubscribe preparation, and trash.
- Persisted review state, sender rules, and action audit live in ignored SQLite database `data/review_state.db`; older JSON files are imported on first startup when the database is empty.
- Review coverage stats are available at `/api/review` and shown in the dashboard.
- Manual moves can save local sender rules that auto-categorize future messages from the same sender.
- Backend monitor service polls on a bounded interval, keeps a bounded cache, classifies results, and exposes `/api/monitor`.
- Unsubscribe action executes only one-click HTTPS unsubscribe headers; other targets are surfaced as review links in the dashboard.
- Backend scan service pages through Gmail metadata, classifies and persists each batch, and exposes `/api/scan`.
- Scan and monitor can opt into AI classification; backend AI calls are chunked with heuristic fallback.
- OpenAI calls now have configurable `max_output_tokens`, retries, pacing, timeout, and classification chunk size so AI scans can iterate under rate limits.
- Trash and one-click unsubscribe actions now preview first and require a short-lived server confirmation token before execution.
- Stored category pages can be loaded from persisted review state for later bulk cleanup beyond the bounded scan cache.
- Mark-read uses Gmail batch modify requests to reduce API calls during large cleanup selections.
- Action audit reads use SQLite rows with bounded JSON payloads for 1000-message cleanup results.
- Mutating API requests are guarded against non-local browser origins.
- Successfully trashed messages are pruned from local review state after the audit entry is recorded.
- Credentials: referenced from files outside the repo:
  - Google OAuth client secret: `GOOGLE_CLIENT_SECRET_FILE`
  - OpenAI key: `OPENAI_API_KEY_FILE`
- Gmail token storage: `data/gmail_token.json` after OAuth.
- Gmail OAuth can be started with `GMAIL_ORGANIZER_OAUTH_REDIRECT_URL=http://localhost:8080/oauth2callback` to match the local Google OAuth client redirect. While the OAuth consent screen is in testing mode, the signing account must be listed as a Google Cloud test user.
- Verified live Gmail OAuth locally on `http://localhost:8080`, fetched 25 messages from `source=gmail`, ran the classification endpoint with AI enabled on that loaded batch, scanned 900 recent Gmail messages, imported 1,004 total stored classifications into SQLite, verified cleanup previews without executing actions, and captured short dashboard/tutorial/cleanup screenshots.

## Next Work

- Add a Gmail history watch path after the local polling path is stable.
- Complete Gmail OAuth in the browser and verify live mailbox fetch/classification.
