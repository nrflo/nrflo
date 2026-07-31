package console

import (
	"errors"
	"fmt"
	"sort"
)

// ErrUnknownProfile is returned by ProfileByName for a non-empty name that
// does not resolve to a built-in profile.
var ErrUnknownProfile = errors.New("console: unknown profile")

// NativeToolPolicy values for Profile.NativeToolPolicy: how the started
// console engine treats its provider's own native tools (independent of the
// console MCP tool catalogue, which BuildRegistry's allowlist controls).
// "none" -> claude gets --tools "" (MCP-only) / codex gets a read-only
// sandbox / the api engine never adds FS regardless of the
// api_native_tools_enabled global. "full" -> unrestricted (today's
// pre-profile behavior). "" behaves like "full" for claude/codex and keeps
// the api engine's existing api_native_tools_enabled gate.
const (
	NativeToolPolicyNone = "none"
	NativeToolPolicyFull = "full"
)

// Profile bundles a named, built-in console-chat configuration: the tool
// catalogue allowlist (BuildRegistry), native-tool policy, default
// model/effort, context budget, refinery default, and system prompt
// template. See CLAUDE.md's Profile Registry section for the invariant this
// enforces at each engine.
type Profile struct {
	Name                string
	DisplayName         string
	Description         string
	DefaultEngine       string // "claude" | "codex" | "api"
	DefaultModelID      string // models registry id
	DefaultEffort       string
	ContextBudgetTokens int
	RefineryDefault     bool
	SystemTemplateID    string
	NativeToolPolicy    string
	// Catalogue is the BuildRegistry allowlist; nil means the full console
	// catalogue (today's pre-profile behavior, e.g. a chat created with no
	// profile).
	Catalogue []string
	// SiblingFlows marks a profile as a valid origin for
	// ChatService.SwitchModel/OpenHandsSibling (chat_service_sibling.go) — a
	// chat running under a profile with this set false (or the zero Profile)
	// refuses those calls with ErrSiblingUnsupportedProfile.
	SiblingFlows bool
}

// t0DeciderCatalogue is the T0 decider's tool allowlist: delegation +
// read/drive access to workflows, sub-workflow plans, findings, tickets,
// artifacts, and web search/consult — deliberately no fs/bash (the profile's
// NativeToolPolicy also locks out the engine's own native tools) and no
// workflow_wait/retry_failed/project_list/project_status (a T0 decider drives
// via delegate/dynamic_workflow, not by running or polling workflows
// directly).
var t0DeciderCatalogue = []string{
	"delegate", "get_delegation", "merge_delegation",
	"workflow_run", "workflow_get", "workflow_list", "workflow_continue", "workflow_stop",
	"dynamic_workflow", "get_subworkflow", "revise_plan", "approve_plan",
	"project_findings_add", "project_findings_add_bulk",
	"project_findings_append", "project_findings_append_bulk",
	"project_findings_get", "project_findings_delete",
	"ticket_create", "ticket_update", "ticket_add_dependency", "ticket_list", "ticket_get", "ticket_current",
	"artifact_list", "artifact_get",
	"web_search", "consult",
}

// t0BareCatalogue is the T0 bare profile's tool allowlist: pure delegation +
// read/drive access to workflows, sub-workflow plans, and tickets — no
// findings, artifacts, web search/consult, or fs/bash. Exactly 14 tools.
var t0BareCatalogue = []string{
	"delegate", "get_delegation", "merge_delegation",
	"dynamic_workflow", "get_subworkflow", "revise_plan", "approve_plan",
	"workflow_run", "workflow_list", "workflow_get", "workflow_continue", "workflow_stop",
	"ticket_list", "ticket_current",
}

// builtinProfiles is the registry populated by init(). Unexported: callers go
// through ProfileByName/ListProfiles so this stays the one source of truth.
var builtinProfiles = map[string]Profile{}

func registerProfile(p Profile) {
	builtinProfiles[p.Name] = p
}

func init() {
	registerProfile(Profile{
		Name:                "t0-decider",
		DisplayName:         "T0 Decider",
		Description:         "Decides, plans, judges, and synthesizes only — delegates all execution. No fs/bash, restricted tool catalogue, tight context budget.",
		DefaultEngine:       "claude",
		DefaultModelID:      "opus-5",
		DefaultEffort:       "xhigh",
		ContextBudgetTokens: 50000,
		RefineryDefault:     true,
		SystemTemplateID:    "tier-t0-decider",
		NativeToolPolicy:    NativeToolPolicyNone,
		Catalogue:           t0DeciderCatalogue,
		SiblingFlows:        true,
	})
	registerProfile(Profile{
		Name:                "t0-hands",
		DisplayName:         "T0 Hands",
		Description:         "Full-tools companion to T0 Decider: executes directly instead of delegating, seeded with the decider's refinery digest.",
		DefaultEngine:       "claude",
		DefaultModelID:      "sonnet-5",
		DefaultEffort:       "",
		ContextBudgetTokens: 150000,
		RefineryDefault:     true,
		SystemTemplateID:    "",
		NativeToolPolicy:    NativeToolPolicyFull,
		Catalogue:           nil,
		SiblingFlows:        true,
	})
	registerProfile(Profile{
		Name:                "t0-bare",
		DisplayName:         "T0 Bare",
		Description:         "Pure-delegation T0: decides and delegates only, with the narrowest tool catalogue of the three t0 profiles.",
		DefaultEngine:       "claude",
		DefaultModelID:      "opus-5",
		DefaultEffort:       "xhigh",
		ContextBudgetTokens: 30000,
		RefineryDefault:     true,
		SystemTemplateID:    "tier-t0-bare",
		NativeToolPolicy:    NativeToolPolicyNone,
		Catalogue:           t0BareCatalogue,
		SiblingFlows:        true,
	})
}

// ProfileByName resolves a built-in profile by name. Empty name is valid and
// resolves to the zero Profile (no catalogue restriction, no defaults) —
// today's pre-profile chat-create behavior.
func ProfileByName(name string) (Profile, error) {
	if name == "" {
		return Profile{}, nil
	}
	p, ok := builtinProfiles[name]
	if !ok {
		return Profile{}, fmt.Errorf("%w: %q", ErrUnknownProfile, name)
	}
	return p, nil
}

// ListProfiles returns every built-in profile, sorted by name.
func ListProfiles() []Profile {
	names := make([]string, 0, len(builtinProfiles))
	for name := range builtinProfiles {
		names = append(names, name)
	}
	sort.Strings(names)
	profiles := make([]Profile, 0, len(names))
	for _, name := range names {
		profiles = append(profiles, builtinProfiles[name])
	}
	return profiles
}
