package api

import (
	"bufio"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/go-chi/chi/v5"
)

// ThemeInfo describes one discovered theme CSS file.
type ThemeInfo struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Swatches []string `json:"swatches,omitempty"` // [bg, surface, accent, highlight]
}

func registerThemes(r chi.Router, d *Deps) {
	r.Get("/themes", listThemes(d))
}

// GET /api/themes — scans static/themes/*.css on every request so themes can
// be added/removed without restarting. Each file declares metadata in header
// comments:
//
//	/* Theme: Display Name */
//	/* id: themekey */
//	/* swatches: #bg #surface #accent #highlight */
func listThemes(d *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		themesDir := filepath.Join(d.BaseDir, "static", "themes")
		entries, err := os.ReadDir(themesDir)
		if err != nil {
			jsonOK(w, []ThemeInfo{{ID: "default", Name: "Default"}})
			return
		}
		var themes []ThemeInfo
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".css") {
				continue
			}
			if info := parseThemeFile(filepath.Join(themesDir, e.Name())); info != nil {
				themes = append(themes, *info)
			}
		}
		if len(themes) == 0 {
			jsonOK(w, []ThemeInfo{{ID: "default", Name: "Default"}})
			return
		}
		sort.Slice(themes, func(i, j int) bool {
			if themes[i].ID == "default" {
				return true
			}
			if themes[j].ID == "default" {
				return false
			}
			return themes[i].Name < themes[j].Name
		})
		jsonOK(w, themes)
	}
}

func parseThemeFile(path string) *ThemeInfo {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	info := &ThemeInfo{}
	scanner := bufio.NewScanner(f)
	for lines := 0; scanner.Scan() && lines < 20; lines++ {
		line := strings.TrimSpace(scanner.Text())
		if v, ok := extractComment(line, "Theme:"); ok {
			info.Name = v
		}
		if v, ok := extractComment(line, "id:"); ok {
			info.ID = v
		}
		if v, ok := extractComment(line, "swatches:"); ok {
			if parts := strings.Fields(v); len(parts) == 4 {
				info.Swatches = parts
			}
		}
		if info.Name != "" && info.ID != "" && len(info.Swatches) == 4 {
			return info
		}
	}
	if info.Name != "" && info.ID != "" {
		return info // swatches optional
	}
	return nil
}

// extractComment parses `/* Key: value */`.
func extractComment(line, key string) (string, bool) {
	line = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(line, "/*"), "*/"))
	if !strings.HasPrefix(line, key) {
		return "", false
	}
	v := strings.TrimSpace(strings.TrimPrefix(line, key))
	return v, v != ""
}
