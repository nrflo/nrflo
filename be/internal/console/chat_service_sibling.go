package console

import (
	"context"
	"errors"
	"time"

	"be/internal/repo"
	"be/internal/ws"
)

// siblingFlushTimeout bounds the origin sidecar flush openSibling performs
// before seeding a sibling chat. Capped so a stalled provider.Run can never
// block sibling creation — a timed-out flush just leaves the seed stale, not
// SwitchModel/OpenHandsSibling failed.
const siblingFlushTimeout = 10 * time.Second

// ErrSiblingUnsupportedProfile is returned by SwitchModel/OpenHandsSibling
// when sid's chat is not running under a profile with SiblingFlows set.
var ErrSiblingUnsupportedProfile = errors.New("console: sibling flow requires a SiblingFlows-enabled origin chat")

// siblingOrigin resolves sid to its live chat session and the console.Profile
// it is running under, and requires that profile to allow the sibling flows
// (Profile.SiblingFlows) — the one gate SwitchModel/OpenHandsSibling share,
// generalized from the old hardcoded t0-decider name-check (Rule 6:
// polymorphism lives in the profile, not the call site).
func (s *ChatService) siblingOrigin(sid string) (*chatSession, Profile, error) {
	origin, ok := s.get(sid)
	if !ok {
		return nil, Profile{}, ErrChatSessionNotFound
	}
	profile, err := ProfileByName(origin.Profile())
	if err != nil || !profile.SiblingFlows {
		return nil, Profile{}, ErrSiblingUnsupportedProfile
	}
	return origin, profile, nil
}

// SwitchModel opens a sibling chat under origin's own profile with a
// different engine/model/effort, seeded with origin sid's refinery digest,
// and leaves sid's own engine live and untouched — a model change must never
// mutate the running engine mid-conversation. Returns the sibling's session
// id.
func (s *ChatService) SwitchModel(sid, engine, modelID, effort string) (string, error) {
	origin, profile, err := s.siblingOrigin(sid)
	if err != nil {
		return "", err
	}
	if engine == "" {
		engine = origin.EngineName()
	}
	return s.openSibling(origin, engine, modelID, effort, profile.Name, "model_switch")
}

// OpenHandsSibling opens a t0-hands sibling chat (full tools, no
// restrictions) seeded with origin sid's refinery digest, and leaves sid's
// own engine live and untouched. Returns the sibling's session id.
func (s *ChatService) OpenHandsSibling(sid string) (string, error) {
	origin, _, err := s.siblingOrigin(sid)
	if err != nil {
		return "", err
	}
	return s.openSibling(origin, "", "", "", "t0-hands", "hands_sibling")
}

// openSibling flushes the origin sidecar's buffered events, then creates a
// new chat under profileName (engine/modelID/effort empty lets the profile's
// own defaults apply — buildChatEngineSpec), seeds it with origin's current
// refinery digest as first-message context (empty when none has folded yet),
// and broadcasts sibling_opened on origin's session channel so a subscribed
// UI can offer to switch to it. The origin chat's engine/session is never
// touched.
func (s *ChatService) openSibling(origin *chatSession, engine, modelID, effort, profileName, reason string) (string, error) {
	s.flushOriginRefinery(origin.id)
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
	if s.deps.Pool != nil {
		_ = repo.NewAgentSessionRepo(s.deps.Pool, s.deps.Clock).SetSiblingOrigin(siblingID, origin.id)
	}

	pushSessionEvent(s.deps.WSHub, origin.id, origin.ProjectID(), ws.EventConsoleChatSiblingOpened, map[string]interface{}{
		"origin_session_id":  origin.id,
		"sibling_session_id": siblingID,
		"reason":             reason,
	})
	return siblingID, nil
}

// flushOriginRefinery best-effort folds sessionID's origin sidecar before a
// sibling reads its digest. Nil-safe (no RefineryMgr wired, matching
// chat_service.go:236 / chat_service_close.go:27) and bounded so a stalled
// fold can never hold up sibling creation.
func (s *ChatService) flushOriginRefinery(sessionID string) {
	if s.deps.RefineryMgr == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), siblingFlushTimeout)
	defer cancel()
	s.deps.RefineryMgr.Flush(ctx, sessionID)
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
