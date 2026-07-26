# Contributing

Thanks for helping improve Gmail Organizer. Focused bug fixes, tests, documentation, accessibility improvements, and well-scoped product changes are welcome.

## Before starting

- Search existing issues and pull requests for related work.
- For a substantial feature or behavior change, open an issue before investing in an implementation.
- Never include real email content, OAuth credentials, tokens, API keys, or local database files in an issue, test fixture, screenshot, or commit.
- Report security problems privately as described in [SECURITY.md](SECURITY.md).

## Local development

Requirements:

- Go 1.25 or newer
- Node.js 24 or newer

Install and verify the frontend:

```powershell
cd web
npm ci
npm test
npm run build
```

Verify the backend from the repository root:

```powershell
go test ./...
go vet ./...
```

Run the application:

```powershell
cd web
npm run build
cd ..
go run ./cmd/server
```

The interface uses demo data when Gmail is not authenticated, so most UI changes do not require access to a real inbox.

## Pull request expectations

- Keep each pull request focused on one problem.
- Explain the user impact and any security or privacy tradeoffs.
- Add or update tests for behavior changes.
- Include a sanitized screenshot for visible UI changes.
- Keep destructive Gmail actions reversible and behind explicit review.
- Preserve loopback-only API boundaries and avoid returning secret values to the browser.
- Run the backend and frontend verification commands before requesting review.

By contributing, you agree that your contribution will be licensed under the repository's MIT License.
