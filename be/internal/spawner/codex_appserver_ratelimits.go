package spawner

import (
	"encoding/json"
	"time"

	"be/internal/service"
	"be/internal/ws"
)

// rateLimitSink is an optional Sink extension for persisting subscription
// rate-limit windows; sinks without a usage store (console chat) omit it.
type rateLimitSink interface {
	UpdateRateLimits(sessionID string, fiveHour, sevenDay service.RateLimitWindow)
}

const (
	codexFiveHourMins = 300
	codexSevenDayMins = 10080
)

type codexRateLimitWindow struct {
	UsedPercent        int    `json:"usedPercent"`
	WindowDurationMins *int64 `json:"windowDurationMins"`
	ResetsAt           *int64 `json:"resetsAt"`
}

type codexRateLimitsParams struct {
	RateLimits struct {
		RateLimitReachedType *json.RawMessage      `json:"rateLimitReachedType"`
		Primary              *codexRateLimitWindow `json:"primary"`
		Secondary            *codexRateLimitWindow `json:"secondary"`
	} `json:"rateLimits"`
}

// rateLimitReached reports whether an account/rateLimits/updated event signals
// an actually-reached limit (rateLimitReachedType != null). The event also
// fires for routine usage updates, which must NOT trigger a restart.
func rateLimitReached(params json.RawMessage) bool {
	var p codexRateLimitsParams
	if json.Unmarshal(params, &p) != nil {
		return false
	}
	return p.RateLimits.RateLimitReachedType != nil && string(*p.RateLimits.RateLimitReachedType) != "null"
}

// recordCodexRateLimits maps the primary/secondary windows of an
// account/rateLimits/updated event onto the 5h/7d store by window duration
// (not by position), skipping windows of any other length.
func recordCodexRateLimits(sessionID string, params json.RawMessage, sink Sink) {
	rs, ok := sink.(rateLimitSink)
	if !ok {
		return
	}
	var p codexRateLimitsParams
	if json.Unmarshal(params, &p) != nil {
		return
	}
	var fiveHour, sevenDay service.RateLimitWindow
	var found bool
	for _, w := range []*codexRateLimitWindow{p.RateLimits.Primary, p.RateLimits.Secondary} {
		if w == nil || w.WindowDurationMins == nil {
			continue
		}
		pct := float64(w.UsedPercent)
		win := service.RateLimitWindow{UsedPercentage: &pct}
		if w.ResetsAt != nil {
			win.ResetsAt = time.Unix(*w.ResetsAt, 0).UTC().Format(time.RFC3339)
		}
		switch *w.WindowDurationMins {
		case codexFiveHourMins:
			fiveHour, found = win, true
		case codexSevenDayMins:
			sevenDay, found = win, true
		}
	}
	if found {
		rs.UpdateRateLimits(sessionID, fiveHour, sevenDay)
	}
}

// UpdateRateLimits persists the windows and broadcasts the same
// agent.rate_limits_updated payload the Claude statusline path emits.
func (ss *spawnerSink) UpdateRateLimits(sessionID string, fiveHour, sevenDay service.RateLimitWindow) {
	if ss.s.config.AgentSvcReal == nil {
		return
	}
	projectID, ticketID, workflowName, err := ss.s.config.AgentSvcReal.UpdateRateLimits(sessionID, fiveHour, sevenDay)
	if err != nil || projectID == "" {
		return
	}
	ss.s.broadcastForSession(ws.EventAgentRateLimitsUpdated, sessionID, projectID, ticketID, workflowName, map[string]interface{}{
		"session_id": sessionID,
		"five_hour":  rateLimitWindowPayload(fiveHour),
		"seven_day":  rateLimitWindowPayload(sevenDay),
	})
}

func rateLimitWindowPayload(w service.RateLimitWindow) map[string]interface{} {
	if w.UsedPercentage == nil {
		return nil
	}
	return map[string]interface{}{"used_percentage": *w.UsedPercentage, "resets_at": w.ResetsAt}
}
