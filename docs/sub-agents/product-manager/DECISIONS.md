# Product Manager Decisions

## 2026-05-27: Cleanup Workbench Goal

Use a guided cleanup workbench as the primary mental model. Do not make users interpret all categories at once in long lanes.

## 2026-05-27: Core User Flows

Unsubscribe:

1. Pick "Ready to unsubscribe" or select unsubscribe-capable messages.
2. Review the selected list and open the email detail when context is needed.
3. Preview unsubscribe, then confirm one-click requests or open review links.

Trash:

1. Pick a cleanup scope such as Promotions, Unwanted, or older mail.
2. Select all recommended items or specific messages.
3. Preview trash, then confirm.

AI review:

1. Choose scope.
2. Run AI Review.
3. Work the "Needs review" queue by accepting category/action recommendations or moving individual messages.

## 2026-05-27: Quick Cleanup Queues

Expose "Ready to unsubscribe" and "Suggested cleanup" as top-level queues above saved categories. These queues speak in outcomes and let the user begin from a cleanup intent instead of first understanding the category model.

## 2026-05-27: Sender Cleanup

Add a sender-grouped unsubscribe queue so the user can work by sender/list instead of selecting individual messages. This supports the product goal of unsubscribing from several senders in a few steps.

## 2026-05-27: AI Suggestions

Expose high-confidence AI/category suggestions as an explicit queue. The user can inspect a row, select individual suggestions, or accept visible suggestions in bulk, keeping AI assistance under user control.
