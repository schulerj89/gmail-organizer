# Security policy

Gmail Organizer handles Gmail metadata, OAuth tokens, optional OpenAI requests, and actions that can change mailbox state. Security reports are welcome and should be handled privately.

## Supported versions

Security fixes are applied to the latest code on the default branch and the latest published `0.x` release.

## Reporting a vulnerability

Use [GitHub private vulnerability reporting](https://github.com/schulerj89/gmail-organizer/security/advisories/new) instead of opening a public issue.

Please include:

- A concise description of the problem and its impact.
- Reproduction steps or a minimal proof of concept.
- The affected commit or release, if known.
- Any suggested mitigation.

Do not include real OAuth credentials, Gmail tokens, API keys, or private email content. You can expect an initial acknowledgment within seven days. Please allow time to validate and prepare a fix before public disclosure.

## Security-sensitive areas

Reports are especially useful for:

- OAuth token or credential exposure.
- Bypasses of loopback or cross-origin request protections.
- Bypasses of destructive-action confirmation tokens.
- Unsafe unsubscribe URL handling or server-side request behavior.
- Exposure of email metadata beyond the intended local boundary.
- Prompt or response handling that leaks more email data than the selected AI workflow requires.
- Unbounded scans, caches, or model requests that create a denial-of-service or cost risk.

## Current trust boundaries

- The Go API binds to loopback by default.
- Gmail and OpenAI credentials are read from local files and are not returned by the API.
- Gmail reads use message metadata and snippets rather than full bodies.
- Destructive actions require a preview and matching short-lived confirmation token.
- Deletion moves messages to Gmail Trash; the permanent-delete API is not used.
- AI classification is optional and uses configurable batch, token, pacing, retry, and timeout limits.
