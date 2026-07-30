package spawner

import "testing"

// csvNamesFSTool: an exact FS-tool name in the CSV is operator intent and
// must bypass the api_native_tools_enabled gate; wildcards and non-FS names
// must not.
func TestCSVNamesFSTool(t *testing.T) {
	cases := []struct {
		csv  string
		want bool
	}{
		{"read_file,bash,findings_add", true},
		{" bash ", true},
		{"findings_add,web_search", false},
		{"*", false},
		{"read_*", false},
		{"", false},
	}
	for _, c := range cases {
		if got := csvNamesFSTool(c.csv); got != c.want {
			t.Errorf("csvNamesFSTool(%q) = %v, want %v", c.csv, got, c.want)
		}
	}
}

// stripFSNames degrades a CSV for spawns with no workdir: FS names drop out,
// everything else keeps its order — the spawn must not die on
// "no tools matched" just because there is nothing to jail bash to.
func TestStripFSNames(t *testing.T) {
	got := stripFSNames("read_file,bash,findings_add,web_search")
	if got != "findings_add,web_search" {
		t.Errorf("stripFSNames = %q, want findings_add,web_search", got)
	}
	if got := stripFSNames("findings_add"); got != "findings_add" {
		t.Errorf("stripFSNames(no-fs) = %q, want unchanged", got)
	}
}
