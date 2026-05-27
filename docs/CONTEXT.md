# Context

## Goal

Build a secure, memory-efficient Gmail AI organizer that categorizes emails, monitors incoming mail, and supports mass delete/unsubscribe workflows from a modern dashboard.

## Current Implementation

- Backend: Go local HTTP API in `cmd/server` and `internal`.
- Frontend: React/Vite app in `web`.
- Dashboard: category lanes, query/max controls, manual refresh, scan, monitoring toggle, AI categorize, manual category moves, bulk read, unsubscribe preparation, and trash.
- Persisted review state and JSONL action audit live under ignored `data/`.
- Review coverage stats are available at `/api/review` and shown in the dashboard.
- Manual moves can save local sender rules that auto-categorize future messages from the same sender.
- Backend monitor service polls on a bounded interval, keeps a bounded cache, classifies results, and exposes `/api/monitor`.
- Unsubscribe action executes only one-click HTTPS unsubscribe headers; other targets are surfaced as review links in the dashboard.
- Backend scan service pages through Gmail metadata, classifies and persists each batch, and exposes `/api/scan`.
- Scan and monitor can opt into AI classification; backend AI calls are chunked with heuristic fallback.
- Trash and one-click unsubscribe actions now preview first and require explicit confirmation before execution.
- Stored category pages can be loaded from persisted review state for later bulk cleanup beyond the bounded scan cache.
- Mark-read uses Gmail batch modify requests to reduce API calls during large cleanup selections.
- Action audit reads support bounded large JSONL entries for 1000-message cleanup results.
- Credentials: referenced from files outside the repo:
  - Google OAuth client secret: `GOOGLE_CLIENT_SECRET_FILE`
  - OpenAI key: `OPENAI_API_KEY_FILE`
- Gmail token storage: `data/gmail_token.json` after OAuth.
- Verified locally with demo fallback data because Gmail OAuth has not yet been completed in this environment.

## Next Work

- Add a Gmail history watch path after the local polling path is stable.
- Complete Gmail OAuth in the browser and verify live mailbox fetch/classification.
