package api

import (
	"bufio"
	"net/http"
	"os"
	"path/filepath"
)

const logsDir = "/tmp/nrflow/logs"
const maxLogLines = 1000

// handleGetLogs serves log file contents as JSON.
// Query param "type" selects be.log or fe.log (default: "be").
// Returns lines in reverse order (latest first), capped at 1000 lines.
func (s *Server) handleGetLogs(w http.ResponseWriter, r *http.Request) {
	logType := r.URL.Query().Get("type")
	if logType == "" {
		logType = "be"
	}
	if logType != "be" && logType != "fe" {
		writeError(w, http.StatusBadRequest, "type must be 'be' or 'fe'")
		return
	}

	filePath := filepath.Join(logsDir, logType+".log")
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

	// Reverse (latest first)
	for i, j := 0, len(lines)-1; i < j; i, j = i+1, j-1 {
		lines[i], lines[j] = lines[j], lines[i]
	}

	// Cap at maxLogLines
	if len(lines) > maxLogLines {
		lines = lines[:maxLogLines]
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"lines": lines,
		"type":  logType,
	})
}
