---
name: subscription-cleanup
description: Review recurring newsletters and subscription email in Outlook, then apply user-approved categories, moves, or deletions.
---

# Subscription cleanup

Search with `mcp__awaken-outlook__list_messages`, using sender names, subjects, or common newsletter terms. Present a bounded candidate list before changing mailbox state.

Use `mcp__awaken-outlook__update_message` or `mcp__awaken-outlook__move_message` only for the candidate set the user approved. Use `mcp__awaken-outlook__delete_message` only after explicit deletion intent. The current Outlook runtime does not click unsubscribe links; do not claim a subscription was cancelled merely because its messages were moved or deleted.
