-- No schema change: layer is stored in the JSON phases column of workflows table.
-- This migration documents the breaking format change from parallel to layer-based phases.
-- Phase format: {"agent": "name", "layer": N, "skip_for": [...]}
-- The "parallel" field is no longer accepted. String-only phase entries are rejected.
SELECT 1;
