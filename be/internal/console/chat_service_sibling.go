package console

import (
	"errors"

	"be/internal/repo"
	"be/internal/ws"
)

// t0DeciderProfile is the one profile name the sibling flows below are gated
// on: SwitchModel/OpenHandsSibling exist because a t0-decider chat must never
// have its live engine mutated mid-conversation (see profiles.go) — for any
// other profile (or no profile) there is no such invariant to protect, so
// these calls are refused rather than silently no-oping.
const t0DeciderProfile = "t0-decider"

// ErrSiblingRequiresT0Decider is returned by SwitchModel/OpenHandsSibling
// when sid's chat is not running under the t0-decider profile.
var ErrSiblingRequiresT0Decider = errors.New("console: sibling flow requires a t0-decider origin chat")

// SwitchModel opens a sibling t0-decider chat under a different
// engine/model/effort, seeded with origin sid's refinery digest, and leaves
// sid's own engine live and untouched — a model change on a t0-decider chat
// must never mutate the running engine mid-conversation. Returns the
// sibling's session id.
func (s *ChatService) SwitchModel(sid, engine, modelID, effort string) (string, error) {
	origin, ok := s.get(sid)
	if !ok {
		return "", ErrChatSessionNotFound
	}
	if origin.Profile() != t0DeciderProfile {
		return "", ErrSiblingRequiresT0Decider
	}
	if engine == "" {
		engine = origin.EngineName()
	}
	return s.openSibling(origin, engine, modelID, effort, t0DeciderProfile, "model_switch")
}

// OpenHandsSibling opens a t0-hands sibling chat (full tools, no
// restrictions) seeded with origin sid's refinery digest, and leaves sid's
// own engine live and untouched. Returns the sibling's session id.
func (s *ChatService) OpenHandsSibling(sid string) (string, error) {
	origin, ok := s.get(sid)
	if !ok {
		return "", ErrChatSessionNotFound
	}
	if origin.Profile() != t0DeciderProfile {
		return "", ErrSiblingRequiresT0Decider
	}
	return s.openSibling(origin, "", "", "", "t0-hands", "hands_sibling")
}

// openSibling creates a new chat under profileName (engine/modelID/effort
// empty lets the profile's own defaults apply — buildChatEngineSpec), seeds
// it with origin's current refinery digest as first-message context (empty
// when none has folded yet), and broadcasts sibling_opened on origin's
// session channel so a subscribed UI can offer to switch to it. The origin
// chat's engine/session is never touched.
func (s *ChatService) openSibling(origin *chatSession, engine, modelID, effort, profileName, reason string) (string, error) {
	digest := s.originDigest(origin.id)

	siblingID, err := s.Create(engine, modelID, effort, origin.ProjectID(), "", profileName, false)
	if err != nil {
		return "", err
	}
	if digest != "" {
		if sib, ok := s.get(siblingID); ok {
			sib.setSeedContext(digest)
		}
	}

	pushSessionEvent(s.deps.WSHub, origin.id, origin.ProjectID(), ws.EventConsoleChatSiblingOpened, map[string]interface{}{
		"origin_session_id":  origin.id,
		"sibling_session_id": siblingID,
		"reason":             reason,
	})
	return siblingID, nil
}

// originDigest reads origin's latest refinery digest content, or "" when
// none has folded yet (or refinery is unconfigured) — never an error, since
// an empty seed is a valid outcome, not a failure.
func (s *ChatService) originDigest(sessionID string) string {
	if s.deps.Pool == nil {
		return ""
	}
	digest, err := repo.NewRefineryDigestRepo(s.deps.Pool, s.deps.Clock).Get(sessionID)
	if err != nil || digest == nil {
		return ""
	}
	return digest.Content
}
