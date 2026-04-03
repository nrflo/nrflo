package api

import (
	"bufio"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

const maxLogLines = 1000

// handleGetLogs serves log file contents as JSON.
// Query param "type" selects be.log or fe.log (default: "be").
// Query param "filter" searches the full file for matching lines (case-insensitive).
// Without filter: returns lines in reverse order (latest first), capped at 1000 lines.
// With filter: returns all matching lines in reverse order (no cap).
func (s *Server) handleGetLogs(w http.ResponseWriter, r *http.Request) {
	logType := r.URL.Query().Get("type")
	if logType == "" {
		logType = "be"
	}
	if logType != "be" && logType != "fe" {
		writeError(w, http.StatusBadRequest, "type must be 'be' or 'fe'")
		return
	}

	filter := r.URL.Query().Get("filter")

	filePath := filepath.Join(s.logsDir, logType+".log")
	f, err := os.Open(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			writeJSON(w, http.StatusOK, map[string]interface{}{
				"lines": []string{},
				"type":  logType,
			})
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to read log file")
		return
	}
	defer f.Close()

	lines := []string{}
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	if filter != "" {
		filterLower := strings.ToLower(filter)
		filtered := []string{}
		for _, line := range lines {
			if strings.Contains(strings.ToLower(line), filterLower) {
				filtered = append(filtered, line)
			}
		}
		lines = filtered
	}

	// Reverse (latest first)
	for i, j := 0, len(lines)-1; i < j; i, j = i+1, j-1 {
		lines[i], lines[j] = lines[j], lines[i]
	}

	// Cap at maxLogLines only when not filtering
	if filter == "" && len(lines) > maxLogLines {
		lines = lines[:maxLogLines]
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"lines": lines,
		"type":  logType,
	})
}
