-- Weighted rotation for tier fallback chains: weight > 0 marks an entry as a
-- rotation candidate; the spawn-time start position is picked by usage
-- deficit against the weights. All-zero weights = strict ordinal (default).
ALTER TABLE tier_models ADD COLUMN weight INTEGER NOT NULL DEFAULT 0;
