package handler

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ExtensionDownloadHandler serves the packaged Chrome extension from the user's
// own instance.
//
// The Settings page used to say "install from the Chrome Web Store", but there
// is no listing and no GitHub release, so self-hosters had no way to get the
// extension short of building it themselves. Serving the zip the app was built
// with keeps the extension and the backend versions in step.
type ExtensionDownloadHandler struct {
	// dir holds built extension artifacts (WXT's .output directory).
	dir string
}

func NewExtensionDownloadHandler(dir string) *ExtensionDownloadHandler {
	return &ExtensionDownloadHandler{dir: dir}
}

// zipPath returns the newest *-chrome.zip in the artifact directory.
func (h *ExtensionDownloadHandler) zipPath() (string, error) {
	if h.dir == "" {
		return "", os.ErrNotExist
	}
	entries, err := os.ReadDir(h.dir)
	if err != nil {
		return "", err
	}
	var names []string
	for _, e := range entries {
		n := e.Name()
		if !e.IsDir() && strings.HasSuffix(n, ".zip") && strings.Contains(n, "chrome") {
			names = append(names, n)
		}
	}
	if len(names) == 0 {
		return "", os.ErrNotExist
	}
	// Highest version sorts last for the usual name-version-chrome.zip shape.
	sort.Strings(names)
	return filepath.Join(h.dir, names[len(names)-1]), nil
}

// Status reports whether a build is available, so the UI can show a real
// download button instead of a link that 404s.
// GET /api/v1/extension/download/status
func (h *ExtensionDownloadHandler) Status(w http.ResponseWriter, r *http.Request) {
	path, err := h.zipPath()
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"available": false,
			"reason":    "no packaged extension found; run `make extension` to build it",
		})
		return
	}
	info, err := os.Stat(path)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"available": false, "reason": "packaged extension is unreadable"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"available": true,
		"filename":  filepath.Base(path),
		"size":      info.Size(),
	})
}

// Download streams the packaged extension zip.
// GET /api/v1/extension/download
func (h *ExtensionDownloadHandler) Download(w http.ResponseWriter, r *http.Request) {
	path, err := h.zipPath()
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"error": "no packaged extension found — build it with `make extension`",
		})
		return
	}

	f, err := os.Open(path)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to open extension archive")
		return
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to stat extension archive")
		return
	}

	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="`+filepath.Base(path)+`"`)
	w.Header().Set("Content-Length", itoa(info.Size()))
	http.ServeContent(w, r, filepath.Base(path), info.ModTime(), f)
	_ = io.Discard
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
