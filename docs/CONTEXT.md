# Context

## Goal

Build a secure, memory-efficient Gmail AI organizer that categorizes emails, monitors incoming mail, and supports mass delete/unsubscribe workflows from a modern dashboard.

## Current Implementation

- Backend: Go local HTTP API in `cmd/server` and `internal`.
- Frontend: React/Vite app in `web`.
- Dashboard: category lanes, query/max controls, manual refresh, monitoring toggle, AI categorize, bulk read, unsubscribe preparation, and trash.
- Persisted review state and JSONL action audit live under ignored `data/`.
- Backend monitor service polls on a bounded interval, keeps a bounded cache, classifies results, and exposes `/api/monitor`.
- Credentials: referenced from files outside the repo:
  - Google OAuth client secret: `GOOGLE_CLIENT_SECRET_FILE`
  - OpenAI key: `OPENAI_API_KEY_FILE`
- Gmail token storage: `data/gmail_token.json` after OAuth.
- Verified locally with demo fallback data because Gmail OAuth has not yet been completed in this environment.

## Next Work

- Add a Gmail history watch path after the local polling path is stable.
- Add real unsubscribe execution for trusted `List-Unsubscribe-Post` flows.
- Complete Gmail OAuth in the browser and verify live mailbox fetch/classification.
