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

## Unsubscribe

The first implementation extracts safe `https://` and `mailto:` List-Unsubscribe targets and prepares them for review. Automatic unsubscribe requests are deferred because blindly opening remote unsubscribe links can confirm account activity to senders.

## AI Classification

The app uses a local heuristic classifier as a deterministic fallback and an OpenAI Responses API classifier when enabled and configured. Prompts include only sender, subject, snippet, and unsubscribe presence to reduce sensitive data exposure.

## Monitoring

The dashboard controls a backend polling service instead of only refreshing in the browser. The monitor keeps a bounded in-memory cache, defaults to local classification to avoid repeated AI calls, and exposes status through `/api/monitor` for UI polling.
