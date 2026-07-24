package tools_builtin

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"unicode/utf8"

	"be/internal/service"
	"be/internal/spawner/apirun"
	"be/internal/spawner/apirun/provider"
	"be/internal/types"
	"be/internal/ws"
)

// findingsFromFileMaxBytes mirrors readFileMaxBytes (fs_read.go): a 256KB
// hard cap on the file content stored as a finding.
const findingsFromFileMaxBytes = 256 << 10

// findingsAddFromFileHandler implements findings_add_from_file: read a file
// from inside the session workdir jail and store its content as a finding,
// so agents can persist evidence without inlining it into the tool call.
type findingsAddFromFileHandler struct{}

func (findingsAddFromFileHandler) Spec() provider.ToolSpec {
	return provider.ToolSpec{
		Name:        "findings_add_from_file",
		Description: "Set a finding key from the contents of a file inside the working directory (max 256KB).",
		InputSchema: json.RawMessage(`{
"type":"object",
"properties":{
"key":{"type":"string","description":"Finding key"},
"path":{"type":"string","description":"File path, relative to the working directory (absolute allowed if inside it)"},
"max_bytes":{"type":"integer","description":"Optional smaller cap on file size, in bytes (hard max 256KB)"}
},
"required":["key","path"],
"additionalProperties":false
}`),
	}
}

func (findingsAddFromFileHandler) Invoke(ctx context.Context, env apirun.ToolEnv, input json.RawMessage) (string, bool, error) {
	var args struct {
		Key      string `json:"key"`
		Path     string `json:"path"`
		MaxBytes int64  `json:"max_bytes"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return invalidArgs(err)
	}
	if args.Key == "" {
		return "key is required", true, nil
	}

	abs, err := resolveFSPath(env, args.Path)
	if err != nil {
		return err.Error(), true, nil
	}
	fi, err := os.Stat(abs)
	if err != nil {
		return err.Error(), true, nil
	}
	if fi.IsDir() {
		return fmt.Sprintf("%s is a directory", args.Path), true, nil
	}

	maxAllowed := int64(findingsFromFileMaxBytes)
	if args.MaxBytes > 0 && args.MaxBytes < maxAllowed {
		maxAllowed = args.MaxBytes
	}
	if fi.Size() > maxAllowed {
		return fmt.Sprintf("file %s (%d bytes) exceeds the %d byte cap", args.Path, fi.Size(), maxAllowed), true, nil
	}

	if env.Findings == nil {
		return missingService("findings")
	}

	data, err := os.ReadFile(abs)
	if err != nil {
		return err.Error(), true, nil
	}
	if !utf8.Valid(data) {
		return fmt.Sprintf("%s is not valid UTF-8 text; findings values must be text", args.Path), true, nil
	}
	sum := sha256.Sum256(data)

	bctx, err := env.Findings.Add(&types.FindingsAddRequest{
		Key:        args.Key,
		Value:      string(data),
		SessionID:  env.SessionID,
		InstanceID: env.WorkflowInstanceID,
	})
	if err != nil {
		return err.Error(), true, nil
	}
	service.BroadcastFromCtx(env.WSHub, ws.EventFindingsUpdated, bctx, map[string]interface{}{
		"agent_type": bctx.AgentType,
		"key":        args.Key,
		"action":     "add-from-file",
	})

	out, marshalErr := json.Marshal(map[string]interface{}{
		"key":    args.Key,
		"bytes":  len(data),
		"sha256": hex.EncodeToString(sum[:]),
	})
	if marshalErr != nil {
		return marshalErr.Error(), true, nil
	}
	return string(out), false, nil
}
