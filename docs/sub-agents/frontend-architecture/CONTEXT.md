# Frontend Architecture Context

## Current Files

- `web/src/main.tsx`: monolithic React dashboard with state, actions, tutorial overlay, lanes, and cards.
- `web/src/styles.css`: custom CSS for toolbar, metrics, lanes, cards, tutorial, and loading overlay.
- `web/src/api.ts`: stable local API wrapper.
- `web/src/types.ts`: shared frontend data types.

## Implementation Direction

- Keep backend unchanged for the first simplification pass.
- Reuse `emails`, `selected`, `reviewStats`, `actionResults`, and `pendingAction`.
- Add UI state for active left-nav category/filter and selected email detail modal.
- Compute visible emails from category/filter instead of rendering every category lane.
- Use CSS grid/flex for a Material-like app shell before adding a heavy component dependency.

