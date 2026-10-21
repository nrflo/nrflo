package spawner

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestCodexFileChangesPersistDiffs(t *testing.T) {
	sink := &testSink{}
	params := json.RawMessage(`{"item":{"type":"fileChange","changes":[{"path":"a.go","kind":{"type":"update"},"diff":"@@ -1 +1 @@\n-old\n+new"},{"path":"b.go","diff":"+second"}]}}`)
	dispatchAppServerEvent("s", rpcEnvelope{Method: "item/completed", Params: params}, sink, 200000, nil)
	if len(sink.recordedMsgs) != 2 {
		t.Fatalf("rows = %v", sink.recordedMsgs)
	}
	row := sink.recordedMsgs[0]
	if row.category != "tool" || !strings.HasPrefix(row.content, "[Edit] a.go") {
		t.Fatalf("file change row = %+v", row)
	}
	var payload struct {
		Input map[string]string `json:"input"`
	}
	if err := json.Unmarshal([]byte(row.payload), &payload); err != nil || payload.Input["diff"] != "@@ -1 +1 @@\n-old\n+new" {
		t.Fatalf("diff payload = %s, err = %v", row.payload, err)
	}
}
