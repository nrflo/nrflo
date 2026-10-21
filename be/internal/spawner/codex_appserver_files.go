package spawner

import "encoding/json"

type appServerFileChange struct {
	Path string `json:"path"`
	Diff string `json:"diff"`
}

func emitFileChanges(sessionID string, changes []appServerFileChange, sink Sink) {
	if len(changes) == 0 {
		sink.BumpLastMessage(sessionID)
		return
	}
	for _, change := range changes {
		input, _ := json.Marshal(map[string]any{"file_path": change.Path, "diff": change.Diff})
		detail := change.Diff
		if len(detail) > maxInlineDetail {
			detail = detail[:maxInlineDetail] + "\n… diff truncated"
		}
		emitMessageWithPayload(sessionID, "[Edit] "+change.Path+"\n"+detail, "tool", BuildToolInvokePayload("", input), sink)
	}
}
