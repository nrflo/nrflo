package spawner

import "encoding/json"

// ingestClaudeTranscriptUsage parses one JSONL Claude transcript line and,
// for an assistant entry carrying a message.usage block, bills it into
// sessionID's running cost via AddSessionCostUsageOnce. Dedup key is the
// entry's top-level uuid (falling back to message.id) so an offset-reset
// re-read of the same line never re-bills; a missing key still bills
// unconditionally rather than dropping usage. Shared by both the workflow
// (ledger_cli.go) and console (console_engine_claude_transcript.go) tailers —
// the sole home for this parse+dedup logic (Rule 6).
func ingestClaudeTranscriptUsage(sessionID string, line []byte) {
	var entry struct {
		Type    string `json:"type"`
		UUID    string `json:"uuid"`
		Message struct {
			ID    string `json:"id"`
			Usage *struct {
				InputTokens         int `json:"input_tokens"`
				CacheReadTokens     int `json:"cache_read_input_tokens"`
				CacheCreationTokens int `json:"cache_creation_input_tokens"`
				OutputTokens        int `json:"output_tokens"`
			} `json:"usage"`
		} `json:"message"`
	}
	if json.Unmarshal(line, &entry) != nil || entry.Type != "assistant" {
		return
	}
	u := entry.Message.Usage
	if u == nil {
		return
	}
	key := entry.UUID
	if key == "" {
		key = entry.Message.ID
	}
	AddSessionCostUsageOnce(sessionID, key, u.InputTokens, u.OutputTokens, u.CacheReadTokens, u.CacheCreationTokens)
}
