package api

import (
	"encoding/json"
	"net/http"
	"os"
	"sort"

	"github.com/go-chi/chi/v5"
)

func registerMock(r chi.Router, d *Deps) {
	r.Get("/mock/scenarios", mockScenarios(d))
}

// GET /api/mock/scenarios — lists scenario keys from test_data.json so the
// edit modal can offer a dropdown for demo trackers.
func mockScenarios(d *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		raw, err := os.ReadFile(d.Fetch.TestDataPath)
		if err != nil {
			jsonOK(w, []string{})
			return
		}
		var scenarios map[string]any
		if err := json.Unmarshal(raw, &scenarios); err != nil {
			jsonOK(w, []string{})
			return
		}
		keys := make([]string, 0, len(scenarios))
		for k := range scenarios {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		jsonOK(w, keys)
	}
}
