package cli

import (
	"strings"
	"testing"
)

func TestFormatLayerFindingsCLI_EmptyMap(t *testing.T) {
	out := formatLayerFindingsCLI(map[string]interface{}{})
	if out != "" {
		t.Errorf("empty map: got %q, want empty string", out)
	}
}

func TestFormatLayerFindingsCLI_NilValue(t *testing.T) {
	m := map[string]interface{}{"analyzer": nil}
	out := formatLayerFindingsCLI(m)
	if !strings.Contains(out, "analyzer:") {
		t.Errorf("output missing agent header: %q", out)
	}
	if !strings.Contains(out, "  _No findings_") {
		t.Errorf("nil value: want \"  _No findings_\" in output, got %q", out)
	}
}

func TestFormatLayerFindingsCLI_EmptyFindingsMap(t *testing.T) {
	m := map[string]interface{}{"builder": map[string]interface{}{}}
	out := formatLayerFindingsCLI(m)
	if !strings.Contains(out, "builder:") {
		t.Errorf("output missing agent header: %q", out)
	}
	if !strings.Contains(out, "  _No findings_") {
		t.Errorf("empty findings map: want \"  _No findings_\" in output, got %q", out)
	}
}

func TestFormatLayerFindingsCLI_WithFindings(t *testing.T) {
	m := map[string]interface{}{
		"builder": map[string]interface{}{
			"status": "done",
			"score":  float64(9),
		},
	}
	out := formatLayerFindingsCLI(m)
	if !strings.Contains(out, "builder:") {
		t.Errorf("output missing agent header: %q", out)
	}
	if !strings.Contains(out, "  status:done") {
		t.Errorf("output missing indented key:value pair: %q", out)
	}
	if !strings.Contains(out, "  score:9") {
		t.Errorf("output missing indented key:value pair: %q", out)
	}
}

func TestFormatLayerFindingsCLI_SortedAgentTypes(t *testing.T) {
	m := map[string]interface{}{
		"z-agent": nil,
		"a-agent": map[string]interface{}{"k": "v"},
		"m-agent": nil,
	}
	out := formatLayerFindingsCLI(m)
	lines := strings.Split(out, "\n")
	var headers []string
	for _, l := range lines {
		if len(l) > 0 && l[0] != ' ' {
			headers = append(headers, l)
		}
	}
	if len(headers) != 3 {
		t.Fatalf("expected 3 agent headers, got %d in: %q", len(headers), out)
	}
	if headers[0] != "a-agent:" || headers[1] != "m-agent:" || headers[2] != "z-agent:" {
		t.Errorf("agent headers not sorted: %v", headers)
	}
}

func TestFormatLayerFindingsCLI_SortedKeys(t *testing.T) {
	m := map[string]interface{}{
		"impl": map[string]interface{}{
			"zkey": "last",
			"akey": "first",
		},
	}
	out := formatLayerFindingsCLI(m)
	akeyIdx := strings.Index(out, "akey:")
	zkeyIdx := strings.Index(out, "zkey:")
	if akeyIdx == -1 || zkeyIdx == -1 {
		t.Fatalf("keys missing in output: %q", out)
	}
	if akeyIdx > zkeyIdx {
		t.Errorf("keys not sorted: akey at %d, zkey at %d in: %q", akeyIdx, zkeyIdx, out)
	}
}

func TestFindingsGetCmd_LayerFlag(t *testing.T) {
	layerFlag := findingsGetCmd.Flags().Lookup("layer")
	if layerFlag == nil {
		t.Fatal("findingsGetCmd missing --layer flag")
	}
	if layerFlag.Value.Type() != "int" {
		t.Errorf("--layer flag type = %q, want \"int\"", layerFlag.Value.Type())
	}
}

func TestFindingsGetCmd_ArgRange(t *testing.T) {
	cmd := findingsGetCmd
	for _, tc := range []struct {
		args    []string
		wantErr bool
	}{
		{[]string{}, false},
		{[]string{"agent-type"}, false},
		{[]string{"agent-type", "key"}, false},
		{[]string{"a", "b", "c"}, true},
	} {
		err := cmd.Args(cmd, tc.args)
		if tc.wantErr && err == nil {
			t.Errorf("args=%v: expected error, got nil", tc.args)
		}
		if !tc.wantErr && err != nil {
			t.Errorf("args=%v: unexpected error: %v", tc.args, err)
		}
	}
}
