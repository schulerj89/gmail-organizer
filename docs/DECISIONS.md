# Decisions

## Stack

Use Go for the backend and React/Vite for the frontend.

Rationale:

- Go has low idle memory, predictable concurrency, and a strong standard library for local HTTP services.
- Gmail cleanup should stream and batch metadata instead of loading entire message bodies into memory.
- A browser dashboard is faster to iterate than a native desktop UI and avoids distributing a Windows installer during early development.

## Secret Handling

API keys and OAuth client secrets are referenced by file path. They are not copied into the repo, serialized to logs, or returned to the UI.

## Gmail Deletion

Bulk delete initially maps to Gmail trash. Permanent deletion is intentionally deferred until the review workflow and audit logging are stronger.

## Manual Review

Manual category moves persist through the same local review state as classifier output. A manual move uses confidence `1.0` and a clear reason so user corrections override later low-confidence classifier output when the email is loaded again.

## Unsubscribe

The app extracts `https://` and `mailto:` List-Unsubscribe targets. It executes only HTTPS one-click unsubscribe requests when Gmail provides `List-Unsubscribe-Post: List-Unsubscribe=One-Click`; ordinary HTTPS and `mailto:` targets are prepared as review links. One-click execution rejects local/private literal IP targets and does not follow redirects.

## AI Classification

The app uses a local heuristic classifier as a deterministic fallback and an OpenAI Responses API classifier when enabled and configured. Prompts include only sender, subject, snippet, and unsubscribe presence to reduce sensitive data exposure.

## Monitoring

The dashboard controls a backend polling service instead of only refreshing in the browser. The monitor keeps a bounded in-memory cache, defaults to local classification to avoid repeated AI calls, and exposes status through `/api/monitor` for UI polling.

## Paged Scanning

Large inbox cleanup uses a scan job that pages through Gmail metadata in batches of up to 200 messages. Each batch is classified and persisted before the next page is fetched, while the UI receives only a bounded recent cache. This keeps memory bounded even when the requested scan limit is much larger than the visible dashboard cache.
