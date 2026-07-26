# Local setup

This guide covers Gmail OAuth, local secret files, and the optional runtime limits used by Gmail Organizer.

## Build and start

From the repository root:

```powershell
cd web
npm ci
npm run build
cd ..
go run ./cmd/server
```

Open <http://127.0.0.1:8787>.

## Google OAuth setup

1. Open Google Cloud Console and create or select a project.
2. Go to **APIs & Services > Library**, find **Gmail API**, and enable it.
3. Configure the **OAuth consent screen**.
4. For a personal Gmail account, choose **External**, keep the app in **Testing**, and add your Gmail address as a test user.
5. Add the `https://www.googleapis.com/auth/gmail.modify` scope if Google asks you to list scopes.
6. Under **APIs & Services > Credentials**, create an OAuth client ID using the **Web application** type.
7. Add the callback URL that matches the port you will run:
   - Default: `http://127.0.0.1:8787/api/auth/google/callback`
   - Alternate example: `http://localhost:8080/oauth2callback`
8. Download the client JSON and keep it outside the repository, or save it as an ignored `client_secret*.json` file.
9. Point Gmail Organizer to it:

```powershell
$env:GOOGLE_CLIENT_SECRET_FILE="<absolute path to client_secret.json>"
go run ./cmd/server
```

If you change scopes, redirect URLs, or OAuth client files after authorizing, delete `data/gmail_token.json` and authorize again so Google can issue a fresh token.

For OAuth client background and redirect URI rules, see [Google's OAuth setup documentation](https://support.google.com/googleapi/answer/6158849).

## Alternate port and callback

If your OAuth client uses `http://localhost:8080/oauth2callback`, start the app with matching settings:

```powershell
$env:GMAIL_ORGANIZER_PORT="8080"
$env:GMAIL_ORGANIZER_OAUTH_REDIRECT_URL="http://localhost:8080/oauth2callback"
go run ./cmd/server
```

Then open <http://localhost:8080>.

## OpenAI classification

OpenAI classification is optional. Store the key in a local file and point the app to that file:

```powershell
$env:OPENAI_API_KEY_FILE="<absolute path to openai_key.txt>"
```

Supported safety and pacing settings:

```powershell
$env:OPENAI_MAX_OUTPUT_TOKENS="2000"
$env:OPENAI_MAX_RETRIES="3"
$env:OPENAI_REQUEST_DELAY_MS="1200"
$env:OPENAI_CLASSIFY_CHUNK_SIZE="25"
$env:OPENAI_TIMEOUT_SECONDS="45"
```

Set `GMAIL_ORGANIZER_ENABLE_OPENAI=false` to disable the OpenAI integration entirely.

## Monitoring and cache limits

```powershell
$env:GMAIL_ORGANIZER_MONITOR_INTERVAL_SECONDS="60"
$env:GMAIL_ORGANIZER_MONITOR_CACHE_LIMIT="500"
$env:GMAIL_ORGANIZER_SCAN_CACHE_LIMIT="1000"
```

Review state, sender rules, and action audit entries are stored in `data/review_state.db`.

## Secret handling

- `.env`, `client_secret*.json`, token files, API-key files, and `data/` must remain untracked.
- Do not paste real email content, OAuth credentials, tokens, or API keys into issues or pull requests.
- The API exposes only safe credential status, never secret values.
