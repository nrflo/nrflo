-- Console yolo: auto-approve console tool calls across claude/api/codex
-- engines. Seeds the global console_yolo gate ON — default-ON read semantics
-- (val != 'false') mean this seed only matters for a future settings UI.

INSERT OR IGNORE INTO config (project_id, key, value) VALUES ('', 'console_yolo', 'true');
