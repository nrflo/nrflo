package service

import "testing"

func TestFindReportFinding_FlatShape(t *testing.T) {
	combined := map[string]any{"report": "flat report"}
	got, ok := FindReportFinding(combined)
	if !ok {
		t.Fatal("ok = false, want true")
	}
	if got != "flat report" {
		t.Errorf("got = %v, want 'flat report'", got)
	}
}

func TestFindReportFinding_GroupedShape(t *testing.T) {
	combined := map[string]any{
		"synthesize:claude:opus_4_8": map[string]any{"report": "grouped report"},
		"analyzer":                   map[string]any{"note": "irrelevant"},
	}
	got, ok := FindReportFinding(combined)
	if !ok {
		t.Fatal("ok = false, want true")
	}
	if got != "grouped report" {
		t.Errorf("got = %v, want 'grouped report'", got)
	}
}

func TestFindReportFinding_NoReport_ReturnsFalse(t *testing.T) {
	combined := map[string]any{"analyzer": map[string]any{"note": "no report here"}}
	if _, ok := FindReportFinding(combined); ok {
		t.Error("ok = true, want false")
	}
}

func TestFindReportFinding_NilMap_ReturnsFalse(t *testing.T) {
	if _, ok := FindReportFinding(nil); ok {
		t.Error("ok = true, want false")
	}
}

func TestExtractReport_StringValue_ReturnedAsIs(t *testing.T) {
	combined := map[string]any{"report": "plain text report"}
	out, err := ExtractReport(combined, "inst-1")
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if out != "plain text report" {
		t.Errorf("out = %q, want 'plain text report'", out)
	}
}

func TestExtractReport_NonStringValue_MarshaledAsIndentedJSON(t *testing.T) {
	combined := map[string]any{"report": map[string]any{"summary": "s", "score": 5.0}}
	out, err := ExtractReport(combined, "inst-1")
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if out == "" {
		t.Error("out is empty, want indented JSON")
	}
}

func TestExtractReport_MissingReport_Errors(t *testing.T) {
	combined := map[string]any{"analyzer": map[string]any{"note": "no report"}}
	if _, err := ExtractReport(combined, "inst-42"); err == nil {
		t.Error("err = nil, want error for missing report")
	}
}
