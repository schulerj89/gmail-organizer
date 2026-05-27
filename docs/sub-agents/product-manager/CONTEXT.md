# Product Manager Context

## User Problem

The current dashboard exposes most implementation controls at once. It works, but it feels cluttered because scanning, stored review state, category lanes, AI jobs, cleanup actions, and system status all compete in the same horizontal surface.

The user wants the app to do more of the lifting: guide them toward cleanup outcomes, keep the page contained, and make high-value actions such as unsubscribe and trash possible in a few steps.

## Product Direction

- Primary surface: an inbox cleanup workbench, not a metrics dashboard.
- Main navigation: left rail for top-level modes.
- Main workflow: choose scope, review AI/category recommendations, act on selected items.
- Email context: detail modal or side panel with subject, sender, snippet, reason, category, unsubscribe capability, and safe actions.
- Progress language: use plain labels such as "Saved emails", "Needs review", "Ready to unsubscribe", and "Cleaned up" instead of storage-oriented labels.

## Useful Research Notes

- Material Design navigation rail guidance supports a compact left-side rail for three to seven top-level destinations on desktop/tablet layouts.
- Gmail search supports date operators such as `after:`, `before:`, `older_than:`, and `newer_than:`; the UI should hide those details behind friendly controls.
- Bulk destructive actions should remain preview-confirm because the app can mutate Gmail state.

