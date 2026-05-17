package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"be/internal/logger"
)

type scriptTransport struct{}

func init() { Register(&scriptTransport{}) }

func (t *scriptTransport) Kind() string { return "script" }

func (t *scriptTransport) Send(n *Notification) error {
	rt := getScriptRuntime()
	if rt == nil {
		return fmt.Errorf("script transport: runtime not initialized")
	}

	code, _ := n.Config["script_code"].(string)
	if code == "" {
		return fmt.Errorf("script transport: script_code not configured")
	}

	ctx := context.Background()

	projectRoot := resolveProjectRoot(rt, n.ProjectID)

	pythonBin := resolvePythonBinForNotify(ctx, rt, n.ProjectID, projectRoot)

	scriptPath, err := writeNotifyScriptFile(rt, n, code)
	if err != nil {
		return err
	}
	defer os.Remove(scriptPath)

	sessionID, spawnToken, err := StartNotifySession(n.ProjectID, n.InstanceID)
	if err != nil {
		logger.Warn(ctx, "script transport: start session failed, continuing without SDK auth", "error", err)
	}

	success := false
	defer func() { EndNotifySession(sessionID, success) }()

	env := buildScriptEnv(rt, n, sessionID, spawnToken)

	runCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(runCtx, pythonBin, scriptPath)
	if projectRoot != "" {
		cmd.Dir = projectRoot
	}
	cmd.Env = env

	var stderrBuf bytes.Buffer
	cmd.Stderr = &stderrBuf

	if runErr := cmd.Run(); runErr != nil {
		snippet := stderrBuf.String()
		if len(snippet) > 2048 {
			snippet = snippet[len(snippet)-2048:]
		}
		if runCtx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("script transport: timeout after 30s: %s", strings.TrimSpace(snippet))
		}
		return fmt.Errorf("script transport: non-zero exit: %s", strings.TrimSpace(snippet))
	}

	success = true
	return nil
}

func resolveProjectRoot(rt *ScriptRuntime, projectID string) string {
	if rt.ProjectRepo == nil || projectID == "" {
		return ""
	}
	proj, err := rt.ProjectRepo.Get(projectID)
	if err != nil || !proj.RootPath.Valid {
		return ""
	}
	return proj.RootPath.String
}

func resolvePythonBinForNotify(ctx context.Context, rt *ScriptRuntime, projectID, projectRoot string) string {
	if rt.VenvMgr != nil && projectRoot != "" && projectID != "" {
		if bin, err := rt.VenvMgr.Ensure(ctx, projectID, projectRoot); err == nil && bin != "" {
			return bin
		}
	}
	return "python3"
}

func writeNotifyScriptFile(rt *ScriptRuntime, n *Notification, code string) (string, error) {
	if rt.NrfloHome != "" && n.DeliveryID != "" {
		dir := filepath.Join(rt.NrfloHome, "notify", "scripts")
		if err := os.MkdirAll(dir, 0o755); err == nil {
			p := filepath.Join(dir, n.DeliveryID+".py")
			if err := os.WriteFile(p, []byte(code), 0o600); err == nil {
				return p, nil
			}
		}
	}
	dir := "/tmp/nrflo/notify"
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("script transport: mkdir: %w", err)
	}
	p := filepath.Join(dir, fmt.Sprintf("%s-%d.py", n.ChannelID, time.Now().UnixNano()))
	if err := os.WriteFile(p, []byte(code), 0o600); err != nil {
		return "", fmt.Errorf("script transport: write script: %w", err)
	}
	return p, nil
}

func buildScriptEnv(rt *ScriptRuntime, n *Notification, sessionID, spawnToken string) []string {
	env := os.Environ()

	// Project env vars (last-wins).
	if rt.EnvVarRepo != nil && n.ProjectID != "" {
		vars, _ := rt.EnvVarRepo.List(n.ProjectID)
		for _, v := range vars {
			env = append(env, v.Name+"="+v.Value)
		}
	}

	// nrflo-controlled vars.
	if rt.SDKDir != "" {
		env = append(env, "NRFLO_SDK_DIR="+rt.SDKDir)
	}
	if rt.NrfloHome != "" {
		env = append(env, "NRFLO_HOME="+rt.NrfloHome)
	}
	if rt.SocketPath != "" {
		env = append(env, "NRFLO_SOCKET="+rt.SocketPath)
	}
	if n.ProjectID != "" {
		env = append(env, "NRFLO_PROJECT="+n.ProjectID)
	}

	// Session vars (only when a session was started).
	if sessionID != "" {
		env = append(env, "NRF_SESSION_ID="+sessionID)
		env = append(env, "NRFLO_AGENT_TOKEN="+spawnToken)
		if n.InstanceID != "" {
			env = append(env, "NRF_WORKFLOW_INSTANCE_ID="+n.InstanceID)
		}
	}

	// Payload and delivery metadata.
	payloadJSON := "{}"
	if n.Payload != nil {
		if b, err := json.Marshal(n.Payload); err == nil {
			payloadJSON = string(b)
		}
	}
	env = append(env, "NRFLO_NOTIFY_PAYLOAD_JSON="+payloadJSON)
	env = append(env, "NRFLO_NOTIFY_DELIVERY_ID="+n.DeliveryID)
	env = append(env, "NRFLO_NOTIFY_CHANNEL_ID="+n.ChannelID)
	env = append(env, "NRFLO_NOTIFY_EVENT_TYPE="+n.EventType)

	return env
}
