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
- Bulk trash, mark-read, and unsubscribe-preparation actions.
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

## Screenshot

![Dashboard screenshot](docs/screenshot.png)

## Security Notes

- The backend binds to `127.0.0.1` by default.
- Email listing fetches metadata headers and snippets, not full message bodies.
- Bulk delete uses Gmail trash, not immediate permanent deletion.
- Unsubscribe actions are prepared for review instead of automatically opening arbitrary remote links.
- API responses include secret file paths and existence status only, never secret contents.

## Verification

```powershell
go test ./...
cd web
npm run build
npm audit --json
```

The current dashboard screenshot was captured from a running local backend at `http://127.0.0.1:8787`.
