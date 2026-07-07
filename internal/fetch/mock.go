package fetch

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/Yata-Dash/Yata-Dash/internal/models"
)

// fetchDemo serves stats from test_data.json for demo/mock trackers.
// No HTTP calls are ever made for these.
func (c *Client) fetchDemo(t models.Tracker) (map[string]any, *Error) {
	raw, err := os.ReadFile(c.TestDataPath)
	if err != nil {
		return nil, errf("mock_read_error", err)
	}
	var scenarios map[string]any
	if err := json.Unmarshal(raw, &scenarios); err != nil {
		return nil, errf("mock_parse_error", err)
	}
	scenario := t.MockScenario
	if scenario == "" {
		scenario = "healthy"
	}
	scRaw, ok := scenarios[scenario]
	if !ok {
		scRaw, ok = scenarios["healthy"]
		if !ok {
			return nil, errf("scenario_not_found", fmt.Errorf("scenario %q not in test data", scenario))
		}
	}
	sc, _ := scRaw.(map[string]any)
	if sc == nil {
		return nil, errf("invalid_scenario", nil)
	}
	stats, _ := sc["stats"].(map[string]any)
	out := map[string]any{}
	for k, v := range stats {
		out[k] = v
	}
	return out, nil
}
