-- Per-model allowed reasoning-effort levels as a JSON array on the model row.
-- Single source of truth for effort validation (service/model_reasoning.go)
-- and the console picker's effort level; replaces the hardcoded
-- xhigh/ultra name-check gating. '[]' means the model offers no effort
-- selection. Matrix verified 2026-07 against `codex app-server model/list`
-- (codex-cli 0.144.4), `claude --help` (--effort low..max), Anthropic
-- output_config docs (effort on 4.6+; haiku has no effort param) and the
-- OpenAI reasoning docs (gpt-5.6 adds max; ultra is codex-only).
ALTER TABLE cli_models ADD COLUMN supported_efforts TEXT NOT NULL DEFAULT '[]';
ALTER TABLE api_models ADD COLUMN supported_efforts TEXT NOT NULL DEFAULT '[]';

-- claude CLI + anthropic API share the Anthropic model capability set.
UPDATE cli_models SET supported_efforts = '["low","medium","high","xhigh","max"]'
WHERE cli_type = 'claude' AND (mapped_model LIKE 'claude-opus-4-7%' OR mapped_model LIKE 'claude-opus-4-8%' OR mapped_model LIKE 'claude-sonnet-5%');
UPDATE cli_models SET supported_efforts = '["low","medium","high","max"]'
WHERE cli_type = 'claude' AND mapped_model LIKE 'claude-opus-4-6%';
UPDATE cli_models SET supported_efforts = '["low","medium","high"]'
WHERE cli_type = 'claude' AND mapped_model LIKE 'claude-haiku%';

UPDATE api_models SET supported_efforts = '["low","medium","high","xhigh","max"]'
WHERE provider = 'anthropic' AND (mapped_model LIKE 'claude-opus-4-7%' OR mapped_model LIKE 'claude-opus-4-8%' OR mapped_model LIKE 'claude-sonnet-5%');
UPDATE api_models SET supported_efforts = '["low","medium","high","max"]'
WHERE provider = 'anthropic' AND mapped_model LIKE 'claude-opus-4-6%';
UPDATE api_models SET supported_efforts = '["low","medium","high"]'
WHERE provider = 'anthropic' AND mapped_model LIKE 'claude-haiku%';

-- codex CLI (from app-server model/list; ultra is Sol/Terra only).
UPDATE cli_models SET supported_efforts = '["low","medium","high","xhigh","max","ultra"]'
WHERE cli_type = 'codex' AND (mapped_model LIKE 'gpt-5.6-sol%' OR mapped_model LIKE 'gpt-5.6-terra%');
UPDATE cli_models SET supported_efforts = '["low","medium","high","xhigh","max"]'
WHERE cli_type = 'codex' AND mapped_model LIKE 'gpt-5.6-luna%';
UPDATE cli_models SET supported_efforts = '["low","medium","high","xhigh"]'
WHERE cli_type = 'codex' AND (mapped_model LIKE 'gpt-5.5%' OR mapped_model LIKE 'gpt-5.4%' OR mapped_model LIKE 'gpt-5.3%');

-- openai Responses API (no ultra; gpt-5.6 adds max).
UPDATE api_models SET supported_efforts = '["low","medium","high","xhigh","max"]'
WHERE provider = 'openai' AND mapped_model LIKE 'gpt-5.6%';
UPDATE api_models SET supported_efforts = '["low","medium","high","xhigh"]'
WHERE provider = 'openai' AND (mapped_model LIKE 'gpt-5.5%' OR mapped_model LIKE 'gpt-5.4%' OR mapped_model LIKE 'gpt-5.3%');

-- Custom rows outside the known matrix: at minimum, the effort the row is
-- already configured with is evidently supported.
UPDATE cli_models SET supported_efforts = json_array(reasoning_effort)
WHERE supported_efforts = '[]' AND reasoning_effort != '';
UPDATE api_models SET supported_efforts = json_array(reasoning_effort)
WHERE supported_efforts = '[]' AND reasoning_effort != '';
