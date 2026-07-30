-- Refinery digests are load-bearing memory: console rotation and sibling
-- chats are told to trust them, and nothing downstream can cross-check a
-- hallucinated digest. Resolve the fold model from the tier-1 chain
-- (fold.go already calls ResolveAgentChain and uses the primary entry)
-- instead of a per-def pin, mirroring _t2_extractor (000220).
UPDATE system_agent_definitions SET
    model = '',
    tier = 1,
    updated_at = datetime('now')
WHERE id = '_refinery';
