package console

import (
	"fmt"
	"os/exec"
)

// lookPath is exec.LookPath by default; tests stub it so Probe never shells
// out (precedent: be/internal/service/cli_availability.go:8).
var lookPath = exec.LookPath

// LaunchInput carries everything a ConsoleDriver needs to prepare a local CLI
// launch. RawModel is the --model flag value as given on the command line;
// MappedModel/ReasoningEffort/FallbackModels are the cli_models registry row's
// values for it, when found — a driver falls back to its adapter's own
// MapModel(RawModel)/GetReasoningEffort(RawModel) when they are empty.
//
// ReasoningEffort matters as much as MappedModel: registry ids are many-to-one
// on mapped_model (CodexAdapter.MapModel sends both codex_gpt55_high and
// codex_gpt55_normal to gpt-5.5), so effort is the ONLY thing distinguishing
// them. Dropping it silently launches a weaker model than the user named.
type LaunchInput struct {
	ServerURL       string
	ProjectID       string
	SessionID       string
	ConsoleToken    string
	ServiceToken    string
	RawModel        string
	MappedModel     string
	ReasoningEffort string
	FallbackModels  string
	WorkDir         string
	NrfloPath       string
}

// LaunchSpec is what the console command execs: argv[0] plus its arguments,
// the full process environment, and the working directory.
type LaunchSpec struct {
	Argv []string
	Env  []string
	Dir  string
}

// ConsoleDriver prepares a native CLI (claude, codex, ...) to be launched as a
// human console session with the nrflo mcp-external bridge injected as an MCP
// server. Provider divergence lives entirely behind this interface — callers
// never branch on the CLI name (Rule 6).
type ConsoleDriver interface {
	// Name is the --cli value this driver serves (e.g. "claude", "codex").
	Name() string
	// Probe checks the CLI binary is installed, returning an actionable error
	// (install hint) when it is not.
	Probe() error
	// Prepare builds the LaunchSpec for exec'ing the CLI and returns a cleanup
	// func that must run after the child process exits.
	Prepare(in LaunchInput) (LaunchSpec, func(), error)
}

// GetDriver returns the ConsoleDriver for a --cli name. This is the only
// provider-name switch in the console package (Rule 6); every other piece of
// per-CLI behavior lives inside the returned driver.
func GetDriver(name string) (ConsoleDriver, error) {
	switch name {
	case "claude":
		return &claudeDriver{}, nil
	case "codex":
		return &codexDriver{}, nil
	default:
		return nil, fmt.Errorf("unknown CLI: %s", name)
	}
}

// bridgeEnv builds the env vars the injected `agent mcp-external` bridge
// needs to adopt the pre-minted console session instead of exchanging a
// service token itself. NRFLO_MCP_TOKEN is included (when set) purely as the
// 401 re-exchange fallback — see agent_mcp_external.go's adopt path.
func bridgeEnv(in LaunchInput) map[string]string {
	env := map[string]string{
		"NRFLO_SERVER_URL":         in.ServerURL,
		"NRFLO_PROJECT":            in.ProjectID,
		"NRFLO_CONSOLE_TOKEN":      in.ConsoleToken,
		"NRFLO_CONSOLE_SESSION_ID": in.SessionID,
	}
	if in.ServiceToken != "" {
		env["NRFLO_MCP_TOKEN"] = in.ServiceToken
	}
	return env
}
