package spawner

import (
	"encoding/json"
	"time"
)

// classifyTranscriptEntry maps a parsed assistant/user block set to the
// timing bucket this entry belongs to: for an assistant entry, precedence
// is tool_use > thinking > text (handles the rare multi-block single-
// timestamp entry; single-block streaming entries are unambiguous); a user
// entry carrying a tool_result attributes to tool wait. ok=false for any
// other content (untimed line — usage-only entries, empty content, etc).
func classifyTranscriptEntry(entryType string, raw json.RawMessage) (bucket TimingBucket, ok bool) {
	var blocks []claudeTranscriptBlock
	if len(raw) == 0 || json.Unmarshal(raw, &blocks) != nil {
		return 0, false
	}
	switch entryType {
	case "assistant":
		hasThinking, hasText := false, false
		for _, b := range blocks {
			switch b.Type {
			case "tool_use":
				return TimingBucketToolArg, true
			case "thinking":
				hasThinking = true
			case "text":
				hasText = true
			}
		}
		if hasThinking {
			return TimingBucketThinking, true
		}
		if hasText {
			return TimingBucketText, true
		}
	case "user":
		for _, b := range blocks {
			if b.Type == "tool_result" {
				return TimingBucketToolWait, true
			}
		}
	}
	return 0, false
}

// recordTranscriptTiming turns one parsed transcript line's top-level
// timestamp + uuid into a dedup-guarded RecordSessionTimingEvent call, when
// the entry classifies into a timing bucket. The delta since the previous
// timed entry is attributed to *this* entry's bucket — i.e. the gap before
// a block (including the model's API latency) rides with that block, since
// the transcript carries no separate latency marker. A zero timestamp
// (missing/unparseable) is dropped rather than corrupting the running
// anchor.
func recordTranscriptTiming(sessionID, entryType string, ts time.Time, uuid string, content json.RawMessage) {
	if ts.IsZero() {
		return
	}
	bucket, ok := classifyTranscriptEntry(entryType, content)
	if !ok {
		return
	}
	RecordSessionTimingEvent(sessionID, uuid, ts, bucket)
}
