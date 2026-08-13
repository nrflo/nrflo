-- ChatNotifier wake-ups ("[nrflo] Delegation … finished") were persisted as
-- category='user_input', so the TUI rendered server-authored turns in the
-- human-input color and the composer's Up-arrow history offered them back as
-- things the user had typed. They now land as 'system_turn'; backfill the
-- rows already written so past chats read correctly and stop polluting the
-- project-scoped input-history aggregate.
--
-- The prefix is server-owned (be/internal/console/chat_notify.go) and anchored
-- at the start, so it cannot match a human message that merely mentions nrflo.

UPDATE agent_messages
SET category = 'system_turn'
WHERE category = 'user_input'
  AND content LIKE '[nrflo] %';
