package apirun

import (
	"context"
	"crypto/sha1"
	"fmt"
	"strings"

	"be/internal/logger"
)

// Tool-result offload ("blob quarantine"): any successful text result larger
// than the configured threshold is stored as a workflow artifact and replaced
// inline by a head+tail excerpt with a pointer, keeping agent context bounded.
// Config keys resolve project > global > built-in default.
const (
	offloadEnabledKey   = "tool_result_offload_enabled"
	offloadThresholdKey = "tool_result_offload_threshold_bytes"

	defaultOffloadThreshold = 8192
	offloadHeadBytes        = 4096
	offloadTailBytes        = 1024
)

// offloadExemptTool reports whether a tool's results always stay inline:
// artifact_get returns content the agent explicitly requested (offloading it
// again would make artifacts unreachable); web_fetch/web_search already bound
// their output via their own excerpt+artifact scheme; read_document's payload
// travels as media blocks; agent_* lifecycle tools are small protocol-critical
// acknowledgements.
func offloadExemptTool(name string) bool {
	switch name {
	case "artifact_get", "web_fetch", "web_search", "read_document":
		return true
	}
	return strings.HasPrefix(name, "agent_")
}

// MaybeOffloadToolResult returns out unchanged when it is under the threshold,
// the tool is exempt, offload is disabled, or the session has no artifact
// scope (nil ArtifactSvc / no workflow instance). Otherwise it stores the full
// payload as a content-addressed artifact and returns a bounded excerpt with a
// pointer. Callers must only pass successful (non-error) results.
func MaybeOffloadToolResult(ctx context.Context, env ToolEnv, toolName, out string) string {
	if offloadExemptTool(toolName) || env.ArtifactSvc == nil || env.WorkflowInstanceID == "" {
		return out
	}
	if offloadSetting(env, offloadEnabledKey, "true") != "true" {
		return out
	}
	threshold := offloadSettingInt(env, offloadThresholdKey, defaultOffloadThreshold)
	if threshold <= 0 || len(out) <= threshold {
		return out
	}

	sum := sha1.Sum([]byte(out))
	name := fmt.Sprintf("toolres_%s_%x.txt", toolName, sum[:8])
	if err := offloadStore(ctx, env, name, out); err != nil {
		logger.Warn(ctx, "tool result offload failed", "tool", toolName, "session", env.SessionID, "error", err)
		return offloadExcerpt(out, fmt.Sprintf(
			"[offloaded excerpt: full output was %d bytes but storing artifact failed (%v); re-run the tool for full output]",
			len(out), err))
	}
	logger.Info(ctx, "tool result offloaded", "tool", toolName, "session", env.SessionID, "bytes", len(out), "artifact", name)
	return offloadExcerpt(out, fmt.Sprintf(
		"[offloaded: full %d-byte output in artifact %q — read it with artifact_get; showing first %dB + last %dB]",
		len(out), name, offloadHeadBytes, offloadTailBytes))
}

// offloadStore writes the artifact, treating a name collision as success:
// names are content-addressed, so an existing artifact with this name already
// holds identical content.
func offloadStore(ctx context.Context, env ToolEnv, name, out string) error {
	_, err := env.ArtifactSvc.AddFromAgent(ctx, env.SessionID, env.ProjectID, env.WorkflowInstanceID, name, "text/plain", []byte(out))
	if err == nil {
		return nil
	}
	existing, lerr := env.ArtifactSvc.List(ctx, env.WorkflowInstanceID)
	if lerr == nil {
		for _, a := range existing {
			if a.Name == name {
				return nil
			}
		}
	}
	return err
}

// offloadExcerpt assembles head + marker + tail on rune boundaries.
func offloadExcerpt(out, marker string) string {
	head := clipRunes(out, offloadHeadBytes)
	tail := out[len(out)-tailStart(out):]
	return head + "\n…" + marker + "\n…" + tail
}

// tailStart returns how many trailing bytes to keep, aligned to a rune start.
func tailStart(s string) int {
	n := offloadTailBytes
	if n > len(s) {
		return len(s)
	}
	start := len(s) - n
	for start < len(s) && !runeStart(s[start]) {
		start++
	}
	return len(s) - start
}

// clipRunes returns the first n bytes of s cut back to a rune boundary.
func clipRunes(s string, n int) string {
	if n >= len(s) {
		return s
	}
	cut := n
	for cut > 0 && !runeStart(s[cut]) {
		cut--
	}
	return s[:cut]
}

func runeStart(b byte) bool { return b&0xC0 != 0x80 }

// offloadSetting resolves a config value project > global > def; nil-safe.
func offloadSetting(env ToolEnv, key, def string) string {
	if env.Pool == nil {
		return def
	}
	if env.ProjectID != "" {
		if v, _ := env.Pool.GetProjectConfig(env.ProjectID, key); v != "" {
			return v
		}
	}
	if v, _ := env.Pool.GetConfig(key); v != "" {
		return v
	}
	return def
}

func offloadSettingInt(env ToolEnv, key string, def int) int {
	v := offloadSetting(env, key, "")
	if v == "" {
		return def
	}
	var n int
	if _, err := fmt.Sscanf(v, "%d", &n); err != nil {
		return def
	}
	return n
}
