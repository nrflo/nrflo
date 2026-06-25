package cli

import (
	"bufio"
	"io"
	"os"
	"regexp"
	"strings"
)

var envNameRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

type envPair struct{ Key, Value string }

// parseDotenv parses KEY=VALUE lines from a .env file. It skips blank lines,
// `#` comments, and malformed lines (no `=`, or a key that isn't a valid env
// name); honors an optional leading `export `; and strips one layer of matching
// single/double quotes around the value. No variable interpolation.
func parseDotenv(r io.Reader) []envPair {
	var out []envPair
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		eq := strings.IndexByte(line, '=')
		if eq <= 0 {
			continue
		}
		key := strings.TrimSpace(line[:eq])
		if !envNameRe.MatchString(key) {
			continue
		}
		out = append(out, envPair{Key: key, Value: unquote(strings.TrimSpace(line[eq+1:]))})
	}
	return out
}

func unquote(s string) string {
	if len(s) >= 2 && (s[0] == '"' || s[0] == '\'') && s[len(s)-1] == s[0] {
		return s[1 : len(s)-1]
	}
	return s
}

// loadDotenv reads path and injects each KEY=VALUE into the process environment
// WITHOUT overriding variables already set (the real process env always wins).
// A missing file is not an error (returns nil, nil). Returns the keys applied.
//
// Loaded at `nrflo_server serve` startup so a gitignored .env in the launch
// directory can supply secrets/config (e.g. EXA_API_KEY, JINA_API_KEY) that the
// server resolves via os.Getenv — the global fallback for per-project secrets.
func loadDotenv(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close() //nolint:errcheck

	var applied []string
	for _, p := range parseDotenv(f) {
		if _, exists := os.LookupEnv(p.Key); exists {
			continue // real env wins over the file
		}
		if err := os.Setenv(p.Key, p.Value); err != nil {
			return applied, err
		}
		applied = append(applied, p.Key)
	}
	return applied, nil
}
