# UX Interaction Decisions

## 2026-05-27: Replace Lanes With Queue

Use one scroll-contained cleanup queue instead of ten category lanes. Category counts remain available as filters in the left nav and summary rail.

## 2026-05-27: Detail Modal

Open a modal for email context instead of expanding cards inline. The modal keeps the page height stable and gives enough room for sender, subject, snippet, category reason, unsubscribe metadata, and actions.

## 2026-05-27: Selection Actions

Show bulk cleanup actions in a sticky selection bar only after at least one message is selected. This reduces idle clutter and makes the next step explicit.

## 2026-05-27: Preview Confirmation Modal

Show unsubscribe and move-to-trash previews in a modal instead of an inline page panel. The modal is the third step in the cleanup flow and should include the affected count, risk note, representative results, and confirm/cancel actions.

## 2026-05-27: Review Decision Modal

Put category correction inside the email detail modal. The user should be able to open one uncertain message, choose a category, optionally save a sender rule, and return to the queue with the item resolved.
