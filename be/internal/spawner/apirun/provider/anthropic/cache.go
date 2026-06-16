package anthropic

import (
	sdk "github.com/anthropics/anthropic-sdk-go"
)

// Anthropic prompt-cache limits. A request may carry at most maxCacheBreakpoints
// cache_control markers; each marker searches at most cacheLookbackBlocks content
// blocks backward for a prior cache entry. cacheMarkerSpacing keeps successive
// message markers comfortably inside that window, so a turn that appends many
// blocks at once (e.g. parallel tool_use/tool_result pairs) cannot push the
// previous turn's marker out of range and miss the cache.
const (
	maxCacheBreakpoints = 4
	cacheLookbackBlocks = 20
	cacheMarkerSpacing  = 15
)

// ephemeralCacheControl returns a 5-minute ephemeral cache marker. The TTL is
// set explicitly on purpose: a zero-valued CacheControlEphemeralParam{} is
// elided by the SDK's `omitzero` json tag, so cache_control would never reach
// the wire and prompt caching would be silently disabled.
func ephemeralCacheControl() sdk.CacheControlEphemeralParam {
	return sdk.CacheControlEphemeralParam{TTL: sdk.CacheControlEphemeralTTLTTL5m}
}

// applyMessageCacheBreakpoints places an ephemeral marker on the last cacheable
// block of the most recent message — the sliding write point the next turn reads
// from — then walks backward, dropping additional markers at message boundaries
// no more than cacheMarkerSpacing blocks apart, until the slot budget is spent.
// Spacing the markers keeps the previous turn's marker inside the
// cacheLookbackBlocks window so incremental cache hits survive turns that append
// many blocks at once. budget is the number of slots left after system/tools.
//
// A single message longer than cacheLookbackBlocks cannot be subdivided here
// (markers attach to message boundaries, not mid-message); that is an accepted
// limitation — the runner appends an assistant + a user message per turn, so the
// boundary between them is itself a placement point.
func applyMessageCacheBreakpoints(msgs []sdk.MessageParam, budget int) {
	if budget <= 0 {
		return
	}

	// Cumulative block end-position of each non-empty message.
	type boundary struct{ idx, end int }
	var bounds []boundary
	total := 0
	for i := range msgs {
		total += len(msgs[i].Content)
		if len(msgs[i].Content) > 0 {
			bounds = append(bounds, boundary{idx: i, end: total})
		}
	}
	if len(bounds) == 0 {
		return
	}

	placed := 0
	lastEnd := bounds[len(bounds)-1].end
	if markLastCacheableBlock(msgs[bounds[len(bounds)-1].idx].Content) {
		placed++
	}
	for j := len(bounds) - 2; j >= 0 && placed < budget; j-- {
		if lastEnd-bounds[j].end < cacheMarkerSpacing {
			continue
		}
		if markLastCacheableBlock(msgs[bounds[j].idx].Content) {
			placed++
			lastEnd = bounds[j].end
		}
	}
}

// markLastCacheableBlock sets an ephemeral marker on the last block of content
// that can carry one, returning false if none can.
func markLastCacheableBlock(content []sdk.ContentBlockParamUnion) bool {
	for i := len(content) - 1; i >= 0; i-- {
		if setBlockCacheControl(&content[i]) {
			return true
		}
	}
	return false
}

// setBlockCacheControl attaches a cache marker to the block's populated variant.
// thinking / redacted_thinking blocks cannot carry cache_control (and never sit
// last in a message), so they report false.
func setBlockCacheControl(b *sdk.ContentBlockParamUnion) bool {
	cc := ephemeralCacheControl()
	switch {
	case b.OfText != nil:
		b.OfText.CacheControl = cc
	case b.OfToolUse != nil:
		b.OfToolUse.CacheControl = cc
	case b.OfToolResult != nil:
		b.OfToolResult.CacheControl = cc
	default:
		return false
	}
	return true
}
