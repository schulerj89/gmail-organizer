# Frontend Architecture Context Folder

This folder holds working context for the Frontend Architecture sub-agent. The current implementation plan lives in `../CONTEXT.md`, and durable decisions live in `../DECISIONS.md`.

## Current Brief

The first redesign pass should keep API contracts stable and reshape the React/CSS surface:

- Reuse existing Gmail, review-store, category, and cleanup endpoints.
- Replace lane rendering with active category state and a single queue.
- Add a modal for email context.
- Use existing CSS and `lucide-react` before adding a UI framework dependency.

