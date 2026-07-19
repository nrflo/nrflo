package spawner

import "encoding/json"

// LedgerKind classifies one context-ledger entry.
type LedgerKind string

const (
	LedgerKindDialog     LedgerKind = "dialog"
	LedgerKindToolUse    LedgerKind = "tool_use"
	LedgerKindToolResult LedgerKind = "tool_result"
	LedgerKindFileRead   LedgerKind = "file_read"
	LedgerKindImage      LedgerKind = "image"
	LedgerKindInjected   LedgerKind = "injected"
)

// LedgerEntry is one ordered block in a session's context ledger. TokensEst
// starts as a bytes/4 heuristic and is reconciled against provider usage when
// available (api mode). Superseded marks an entry a later dedup-matching
// entry has replaced; superseded entries are excluded from epoch totals but
// kept in the snapshot for inspection.
type LedgerEntry struct {
	ID          string     `json:"id"`
	Kind        LedgerKind `json:"kind"`
	TokensEst   int        `json:"tokens_est"`
	BornTurn    int        `json:"born_turn"`
	LastRefTurn int        `json:"last_ref_turn"`
	Source      string     `json:"source"`
	SHA         string     `json:"sha,omitempty"`
	Superseded  bool       `json:"superseded"`
	Approx      bool       `json:"approx"`
}

// ContextLedgerSnapshot is a read-only view of one session's context ledger,
// returned by LedgerSnapshot for the debug endpoint.
type ContextLedgerSnapshot struct {
	SessionID    string             `json:"session_id"`
	Entries      []LedgerEntry      `json:"entries"`
	TotalsByKind map[LedgerKind]int `json:"totals_by_kind"`
}

// LedgerEpochSummary is the debounced WS broadcast payload: totals by kind
// plus a grand total, omitting individual entries.
type LedgerEpochSummary struct {
	SessionID    string
	TotalTokens  int
	EntryCount   int
	TotalsByKind map[LedgerKind]int
}

// toolCallMeta correlates a tool_use block with the tool name / path hint
// its later tool_result is classified against (file_read vs generic
// tool_result), keyed per-engine: api by ToolUseID, cli/codex by tool name
// (codex has no call id; invoke/result for one call are always dispatched
// synchronously back-to-back).
type toolCallMeta struct {
	name string
	path string
}

// estTokens applies the bytes/4 token-estimate heuristic shared by all three
// ledger writers; api mode additionally reconciles it against provider usage
// (ledger.reconcileUsage).
func estTokens(nbytes int) int {
	if nbytes <= 0 {
		return 0
	}
	return nbytes / 4
}

// isReadToolName reports whether name identifies a file-read tool (builtin,
// claude-native, or MCP-bridged), so its tool_result classifies as file_read
// instead of a generic tool_result.
func isReadToolName(name string) bool {
	switch name {
	case "read_file", "read_document", "Read":
		return true
	}
	n := len(name)
	for _, suffix := range []string{"__read_file", "__read_document", "__Read"} {
		if n > len(suffix) && name[n-len(suffix):] == suffix {
			return true
		}
	}
	return false
}

// extractPathHint pulls a path-ish field out of a tool_use Input payload
// (path/file_path/name, checked in that order) so a tool_result can be
// classified as file_read and correlated across later references.
func extractPathHint(input json.RawMessage) string {
	if len(input) == 0 {
		return ""
	}
	var v struct {
		Path     string `json:"path"`
		FilePath string `json:"file_path"`
		Name     string `json:"name"`
	}
	if json.Unmarshal(input, &v) != nil {
		return ""
	}
	if v.Path != "" {
		return v.Path
	}
	if v.FilePath != "" {
		return v.FilePath
	}
	return v.Name
}
