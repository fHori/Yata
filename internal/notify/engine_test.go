package notify

import (
	"strings"
	"testing"

	"github.com/Yata-Dash/Yata-Dash/internal/models"
)

func merged(fields map[string]any) models.MergedStats {
	m := models.MergedStats{}
	for k, v := range fields {
		m[k] = models.StatField{Value: v}
	}
	return m
}

func TestEvalCondition(t *testing.T) {
	m := merged(map[string]any{
		"ratio": "0.58", "buffer": "500 GiB", "warnings": "2", "active_event": "Global Freeleech",
	})
	cur := rawValues(m)
	prev := map[string]string{"group": "Member"}
	curWithGroup := map[string]string{"group": "Power User"}

	cases := []struct {
		name string
		c    models.Condition
		cur  map[string]string
		reach bool
		want bool
	}{
		{"ratio below threshold", models.Condition{Field: "ratio", Op: "lt", Value: "1.0"}, cur, true, true},
		{"ratio not above", models.Condition{Field: "ratio", Op: "gt", Value: "1.0"}, cur, true, false},
		{"buffer size compare GiB<TiB", models.Condition{Field: "buffer", Op: "lt", Value: "1 TiB"}, cur, true, true},
		{"warnings gt 0", models.Condition{Field: "warnings", Op: "gt", Value: "0"}, cur, true, true},
		{"freeleech active", models.Condition{Field: "freeleech_active", Op: "is_true"}, cur, true, true},
		{"reachable is_false when up", models.Condition{Field: "reachable", Op: "is_false"}, cur, true, false},
		{"reachable is_false when down", models.Condition{Field: "reachable", Op: "is_false"}, cur, false, true},
		{"group changed", models.Condition{Field: "group", Op: "changed"}, curWithGroup, true, true},
		{"group unchanged", models.Condition{Field: "group", Op: "changed"}, prev, true, false},
	}
	for _, tc := range cases {
		if got := evalCondition(tc.c, m, tc.cur, prev, tc.reach); got != tc.want {
			t.Errorf("%s: got %v want %v", tc.name, got, tc.want)
		}
	}
}

func TestEvalRuleMatchModes(t *testing.T) {
	m := merged(map[string]any{"ratio": "0.58", "warnings": "0"})
	cur := rawValues(m)
	prev := map[string]string{}

	ratioLow := models.Condition{Field: "ratio", Op: "lt", Value: "1.0"}
	warnHigh := models.Condition{Field: "warnings", Op: "gt", Value: "0"}

	all := models.AlertRule{Match: "all", Conditions: []models.Condition{ratioLow, warnHigh}}
	if evalRule(all, m, cur, prev, true) {
		t.Error("AND rule should be false (warnings not > 0)")
	}
	any := models.AlertRule{Match: "any", Conditions: []models.Condition{ratioLow, warnHigh}}
	if !evalRule(any, m, cur, prev, true) {
		t.Error("OR rule should be true (ratio < 1.0)")
	}
}

func TestEventDescriptionCarriesTextAndEnds(t *testing.T) {
	m := merged(map[string]any{
		"active_event":         "🌐 Global freeleech mode activated",
		"active_event_ends_at": float64(1772738619), // JSON numbers arrive as float64
	})
	got := eventDescription(m)
	if !strings.Contains(got, "Global freeleech mode activated") {
		t.Errorf("event text missing: %q", got)
	}
	if !strings.Contains(got, "(ends ") {
		t.Errorf("end time missing: %q", got)
	}

	// And the full rule description should use it for a freeleech is_true match.
	rule := models.AlertRule{Match: "all", Conditions: []models.Condition{{Field: "freeleech_active", Op: "is_true"}}}
	desc := describeMatch(rule, m, rawValues(m), map[string]string{}, true)
	if !strings.Contains(desc, "Global freeleech mode activated") || !strings.Contains(desc, "(ends ") {
		t.Errorf("describeMatch should carry event text + ends, got %q", desc)
	}
}

// fakeCfg implements ConfigSource for the priming test.
type fakeCfg struct{ n models.NotificationConfig }

func (f fakeCfg) Notifications() models.NotificationConfig { return f.n }

func TestPrimingSuppressesFirstEval(t *testing.T) {
	rule := models.AlertRule{
		ID: "r1", Name: "low ratio", Enabled: true, Match: "all",
		Conditions:   []models.Condition{{Field: "ratio", Op: "lt", Value: "1.0"}},
		Destinations: []string{"d1"},
	}
	// No real destination → Send would fail, but priming must not even attempt
	// to fire. The destination is disabled so resolveDestinations is empty too.
	eng := New(fakeCfg{n: models.NotificationConfig{Rules: []models.AlertRule{rule}}}, nil)
	tr := models.Tracker{ID: "t1", Name: "T"}
	m := merged(map[string]any{"ratio": "0.5"})

	eng.Evaluate(tr, m, true) // priming pass
	if !eng.primed["t1"] {
		t.Fatal("tracker should be primed after first eval")
	}
	if !eng.firing["r1|t1"] {
		t.Fatal("firing state should record matched=true during priming")
	}
}

// TestUnreadMailEdgeTrigger walks the full unread-mail alert cycle: priming,
// mail arriving (fires), sitting unread (no re-fire), being read (re-arms).
func TestUnreadMailEdgeTrigger(t *testing.T) {
	rule := models.AlertRule{ID: "r1", Name: "Mail", Enabled: true, Match: "all",
		Conditions: []models.Condition{{Field: "unread_mail", Op: "is_true"}}}
	eng := New(fakeCfg{n: models.NotificationConfig{Rules: []models.AlertRule{rule}}}, nil)
	tr := models.Tracker{ID: "t1", Name: "T"}

	eng.Evaluate(tr, merged(map[string]any{"unread_mail": "false"}), true) // prime: all read
	if eng.firing["r1|t1"] {
		t.Fatal("no unread mail must not match")
	}
	eng.Evaluate(tr, merged(map[string]any{"unread_mail": "true"}), true) // mail arrives → rising edge
	if !eng.firing["r1|t1"] {
		t.Fatal("unread mail appearing must match (rising edge fires)")
	}
	eng.Evaluate(tr, merged(map[string]any{"unread_mail": "false"}), true) // read → clears, re-arms
	if eng.firing["r1|t1"] {
		t.Fatal("reading the mail must clear the match")
	}
	// Field absent entirely (unknown layout) → not true → still clear.
	eng.Evaluate(tr, merged(map[string]any{"ratio": "1.0"}), true)
	if eng.firing["r1|t1"] {
		t.Fatal("unknown/unset flag must never read as unread")
	}
}
