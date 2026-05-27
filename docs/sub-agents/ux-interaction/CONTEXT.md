# UX Interaction Context

## Current UX Issues

- Category lanes create a wide, long page that hides later content and makes comparisons hard.
- The user cannot inspect enough email context without reading compact cards.
- Labels such as "Persisted review state" expose implementation details.
- Bulk actions are always present even when the next best action depends on selection.
- Gmail query syntax is still visible in generated previews and should remain secondary.

## Proposed Interaction Model

- Left nav: Cleanup, Review, Subscriptions, Rules, Settings.
- Header: connection status and tutorial only.
- Scope bar: friendly date filter, optional extra search, visible count, scan/monitor controls grouped under "Coverage".
- Main list: single message table/list filtered by selected nav/category.
- Detail modal: opens from a row; supports select, move, unsubscribe preview, trash preview.
- Sticky action bar: appears when messages are selected; offers Mark read, Unsubscribe, Trash, and Move.

