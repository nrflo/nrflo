package id

import (
	"crypto/rand"
	"encoding/hex"
	"strings"

	"github.com/google/uuid"
)

// MintToken returns a 32-byte random hex token used as a bearer credential
// (agent_sessions.spawn_token). Shared by the spawner (agent envelope) and
// the console/observer session services.
func MintToken() string {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		// crypto/rand failure is essentially impossible on supported platforms;
		// fall back to a UUID so we never panic. Token uniqueness is the only
		// invariant — uniqueness via UUIDv4 is sufficient.
		return strings.ReplaceAll(uuid.New().String(), "-", "") +
			strings.ReplaceAll(uuid.New().String(), "-", "")
	}
	return hex.EncodeToString(buf)
}
