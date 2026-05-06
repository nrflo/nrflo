package api

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"be/internal/service"
)

type browseEntry struct {
	Name       string    `json:"name"`
	IsDir      bool      `json:"is_dir"`
	IsPython   bool      `json:"is_python"`
	Size       int64     `json:"size"`
	ModifiedAt time.Time `json:"modified_at"`
}

type browseResponse struct {
	Path    string        `json:"path"`
	Entries []browseEntry `json:"entries"`
}

type readFileResponse struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

const maxPythonScriptFileSize = 1 << 20 // 1 MiB

// handleBrowsePythonScriptDir lists directory contents filtered to dirs and .py files.
// GET /api/v1/python-scripts/browse?path=
func (s *Server) handleBrowsePythonScriptDir(w http.ResponseWriter, r *http.Request) {
	projectID := getProjectID(r)
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "X-Project header required")
		return
	}

	dirPath := r.URL.Query().Get("path")
	if dirPath == "" {
		// Default: project root_path, else user home dir.
		defaultPath, err := s.browseDefaultPath(projectID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		dirPath = defaultPath
	}

	if !filepath.IsAbs(dirPath) {
		writeError(w, http.StatusBadRequest, "path must be absolute")
		return
	}
	dirPath = filepath.Clean(dirPath)

	info, err := os.Stat(dirPath)
	if err != nil {
		if os.IsNotExist(err) {
			writeError(w, http.StatusNotFound, "path does not exist")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !info.IsDir() {
		writeError(w, http.StatusBadRequest, "path is not a directory")
		return
	}

	entries, err := os.ReadDir(dirPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	result := browseResponse{
		Path:    dirPath,
		Entries: []browseEntry{},
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}
		isDir := e.IsDir()
		isPython := !isDir && strings.HasSuffix(e.Name(), ".py")
		if !isDir && !isPython {
			continue
		}
		fi, err := e.Info()
		if err != nil {
			continue
		}
		result.Entries = append(result.Entries, browseEntry{
			Name:       e.Name(),
			IsDir:      isDir,
			IsPython:   isPython,
			Size:       fi.Size(),
			ModifiedAt: fi.ModTime().UTC(),
		})
	}

	writeJSON(w, http.StatusOK, result)
}

// handleReadPythonScriptFile returns the content of a .py file.
// GET /api/v1/python-scripts/read-file?path=
func (s *Server) handleReadPythonScriptFile(w http.ResponseWriter, r *http.Request) {
	projectID := getProjectID(r)
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "X-Project header required")
		return
	}

	filePath := r.URL.Query().Get("path")
	if filePath == "" {
		writeError(w, http.StatusBadRequest, "path is required")
		return
	}
	if !filepath.IsAbs(filePath) {
		writeError(w, http.StatusBadRequest, "path must be absolute")
		return
	}
	filePath = filepath.Clean(filePath)

	if !strings.HasSuffix(filePath, ".py") {
		writeError(w, http.StatusBadRequest, "path must end in .py")
		return
	}

	info, err := os.Stat(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			writeError(w, http.StatusNotFound, "file does not exist")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !info.Mode().IsRegular() {
		writeError(w, http.StatusBadRequest, "path must be a regular file")
		return
	}
	if info.Size() > maxPythonScriptFileSize {
		writeError(w, http.StatusRequestEntityTooLarge, "file exceeds 1 MiB limit")
		return
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, readFileResponse{
		Path:    filePath,
		Content: string(data),
	})
}

// browseDefaultPath returns the project root_path when set, otherwise the user's home dir.
func (s *Server) browseDefaultPath(projectID string) (string, error) {
	svc := service.NewProjectService(s.pool, s.clock)
	project, err := svc.Get(projectID)
	if err == nil && project.RootPath.Valid && project.RootPath.String != "" {
		return project.RootPath.String, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return home, nil
}
