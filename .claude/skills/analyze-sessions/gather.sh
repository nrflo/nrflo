#!/usr/bin/env bash
# Read-only evidence dump for /analyze-sessions: the last N terminal workflow
# instances across ALL projects, their session anomalies, error messages,
# final-result findings, and recent server log errors.
# Usage: gather.sh [N]   (default 20; writes the report to stdout)
set -euo pipefail

N="${1:-20}"
DB="${NRFLO_HOME:-$HOME/.nrflo}/nrflo.data"
LOG="${NRFLO_HOME:-$HOME/.nrflo}/logs/be.log"

if [ ! -f "$DB" ]; then
  echo "ERROR: nrflo DB not found at $DB (set NRFLO_HOME?)" >&2
  exit 1
fi

q() { sqlite3 -readonly -separator ' | ' "$DB" "$1"; }

# Shared CTE: the N most recently finished instances, any project.
RECENT="WITH recent AS (
  SELECT id, project_id, ticket_id, workflow_id, scope_type, status,
         retry_count, subworkflow_depth, created_at, updated_at
  FROM workflow_instances
  WHERE status IN ('completed','failed','project_completed')
  ORDER BY updated_at DESC LIMIT $N
)"

echo "=== nrflo session analysis: last $N finished workflows (all projects) ==="
echo "DB: $DB   generated: $(date -u '+%Y-%m-%dT%H:%M:%SZ')"

echo
echo "--- [1] instances (newest first; sub>0 = subworkflow) ---"
q "$RECENT SELECT substr(id,1,8), project_id, ticket_id, workflow_id, status,
     retry_count, subworkflow_depth, updated_at
   FROM recent;"

echo
echo "--- [2] anomaly aggregate across the set ---"
q "$RECENT SELECT
     count(*)                                            || ' sessions, ' ||
     sum(status='failed')                                || ' failed, ' ||
     sum(result='fail')                                  || ' result=fail, ' ||
     sum(result='continue')                              || ' continues, ' ||
     sum(restart_count)                                  || ' restarts, ' ||
     sum(nudge_count)                                    || ' nudges, ' ||
     sum(stop_block_count)                               || ' stop-blocks, ' ||
     sum(rate_limit_retry_count)                         || ' rate-limit retries, ' ||
     sum(coalesce(context_left,100) < 15)                || ' low-context (<15%)'
   FROM agent_sessions WHERE workflow_instance_id IN (SELECT id FROM recent);"

echo
echo "--- [3] anomalous sessions (failed / fail / restarts / nudges / stop-blocks / rate-limits / low-context) ---"
q "$RECENT SELECT r.project_id, r.ticket_id, s.phase, s.agent_type, s.effective_mode,
     coalesce(s.model_id,''), s.status, coalesce(s.result,''),
     'rst='||s.restart_count, 'ndg='||s.nudge_count, 'stp='||s.stop_block_count,
     'rl='||s.rate_limit_retry_count, 'ctx='||coalesce(s.context_left,-1),
     coalesce(s.last_retry_class,''),
     replace(substr(coalesce(s.result_reason,''),1,160),char(10),' ')
   FROM agent_sessions s JOIN recent r ON r.id = s.workflow_instance_id
   WHERE s.status='failed' OR s.result='fail' OR s.restart_count>0
      OR s.nudge_count>0 OR s.stop_block_count>0 OR s.rate_limit_retry_count>0
      OR coalesce(s.context_left,100) < 15
   ORDER BY r.updated_at DESC;"

echo
echo "--- [4] error messages + failed gate commands (last 3 per session) ---"
q "$RECENT SELECT project_id, ticket_id, phase, seq, category, msg FROM (
     SELECT r.project_id, r.ticket_id, s.phase, m.seq, m.category,
            replace(substr(m.content,1,200),char(10),' ') AS msg,
            row_number() OVER (PARTITION BY m.session_id ORDER BY m.seq DESC) AS rn,
            r.updated_at
     FROM agent_messages m
     JOIN agent_sessions s ON s.id = m.session_id
     JOIN recent r ON r.id = s.workflow_instance_id
     WHERE m.category = 'error'
        OR (m.category = 'validation' AND m.content LIKE '%exit=%'
            AND m.content NOT LIKE '%exit=0%')
   ) WHERE rn <= 3 ORDER BY updated_at DESC, seq;"

echo
echo "--- [5] final results + review summaries (truncated) ---"
q "$RECENT SELECT r.project_id, r.ticket_id, f.key,
     replace(substr(f.value,1,400),char(10),' ')
   FROM findings f JOIN recent r ON r.id = f.workflow_instance_id
   WHERE f.key IN ('workflow_final_result','review_summary')
   ORDER BY r.updated_at DESC, f.key;"

echo
echo "--- [6] be.log errors/warnings since the oldest instance in the set (last 120 lines) ---"
OLDEST=$(q "$RECENT SELECT substr(min(updated_at),1,10) FROM recent;")
if [ -f "$LOG" ]; then
  # be.log format: "YYYY-MM-DD HH:MM:SS LEVEL ..." — match the level field
  # only, so message text like "errors=0" doesn't false-positive.
  awk -v d="$OLDEST" '$0 >= d && $3 ~ /^(ERROR|WARN)$/' "$LOG" 2>/dev/null \
    | tail -120 || true
else
  echo "(no be.log at $LOG)"
fi

echo
echo "=== end of report ==="
