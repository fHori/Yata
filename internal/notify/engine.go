package notify

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Yata-Dash/Yata-Dash/internal/models"
	"github.com/Yata-Dash/Yata-Dash/internal/parse"
)

// Logger is the subset of the app logger the engine uses (avoids an import cycle).
type Logger interface {
	Infof(string, ...any)
	Warnf(string, ...any)
	Debugf(string, ...any)
}

// ConfigSource provides the current notification config to the engine.
type ConfigSource interface {
	Notifications() models.NotificationConfig
}

// numericFields are stat fields exposed to conditions as numbers, with the unit
// used for comparison. Sizes compare in GiB, durations in days.
var numericFields = map[string]string{
	"ratio":         "",
	"buffer":        "GiB",
	"uploaded":      "GiB",
	"downloaded":    "GiB",
	"seed_size":     "GiB",
	"seeding":       "",
	"leeching":      "",
	"warnings":      "",
	"hit_and_runs":  "",
	"bonus_points":  "",
	"avg_seed_time": "days",
}

// Engine evaluates alert rules against fresh stats and fires webhooks on the
// rising edge (false→true) of a rule. State is kept in memory; the first
// evaluation per tracker after start "primes" silently so a restart never
// re-fires conditions that are already true.
type Engine struct {
	cfg ConfigSource
	log Logger

	mu        sync.Mutex
	firing    map[string]bool      // "ruleID|trackerID" → currently matched
	lastFired map[string]time.Time // "ruleID|trackerID" → last notification time
	prevVals  map[string]map[string]string // trackerID → field → previous raw value
	primed    map[string]bool      // trackerID → baseline established
}

// New creates an alert engine.
func New(cfg ConfigSource, log Logger) *Engine {
	return &Engine{
		cfg:       cfg,
		log:       log,
		firing:    map[string]bool{},
		lastFired: map[string]time.Time{},
		prevVals:  map[string]map[string]string{},
		primed:    map[string]bool{},
	}
}

// Evaluate runs every rule for one tracker against its merged stats. reachable
// reports whether the latest fetch succeeded (drives the `reachable` field).
func (e *Engine) Evaluate(t models.Tracker, merged models.MergedStats, reachable bool) {
	cfg := e.cfg.Notifications()
	if len(cfg.Rules) == 0 {
		return
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	prev := e.prevVals[t.ID]
	if prev == nil {
		prev = map[string]string{}
	}
	cur := rawValues(merged)
	primed := e.primed[t.ID]

	for _, rule := range cfg.Rules {
		if !rule.Enabled || len(rule.Conditions) == 0 {
			continue
		}
		if !rule.Matches(t.ID) {
			continue
		}
		matched := evalRule(rule, merged, cur, prev, reachable)
		key := rule.ID + "|" + t.ID
		if primed && matched && !e.firing[key] {
			e.fire(cfg, rule, t, merged, cur, prev, reachable, key)
		}
		e.firing[key] = matched
	}

	// Update previous-value snapshot + mark primed for next time.
	e.prevVals[t.ID] = cur
	e.primed[t.ID] = true
}

// Announce immediately fires the given (typically newly-created) rules for any
// in-scope tracker that ALREADY meets their conditions, using last-known merged
// stats. This gives the user instant confirmation on setup instead of waiting
// for a future transition. It records firing state so the normal edge-triggered
// evaluation won't re-fire the same already-true condition.
func (e *Engine) Announce(rules []models.AlertRule, trackers []models.Tracker, mergedFn func(string) models.MergedStats) {
	cfg := e.cfg.Notifications()
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, rule := range rules {
		if !rule.Enabled || len(rule.Conditions) == 0 {
			continue
		}
		for _, t := range trackers {
			if !t.Enabled || !rule.Matches(t.ID) {
				continue
			}
			merged := mergedFn(t.ID)
			cur := rawValues(merged)
			prev := e.prevVals[t.ID]
			if prev == nil {
				prev = map[string]string{}
			}
			matched := evalRule(rule, merged, cur, prev, true)
			key := rule.ID + "|" + t.ID
			if matched && !e.firing[key] {
				e.fire(cfg, rule, t, merged, cur, prev, true, key)
			}
			// Record state but DON'T mark the tracker primed — let normal
			// priming baseline the OTHER (existing) rules silently.
			e.firing[key] = matched
		}
	}
}

// fire sends the rule's message to its destinations (respecting cooldown).
func (e *Engine) fire(cfg models.NotificationConfig, rule models.AlertRule, t models.Tracker,
	merged models.MergedStats, cur, prev map[string]string, reachable bool, key string) {
	if cd := time.Duration(rule.CooldownMins) * time.Minute; cd > 0 {
		if last, ok := e.lastFired[key]; ok && time.Since(last) < cd {
			return
		}
	}
	title := fmt.Sprintf("Yata alert: %s", rule.Name)
	msg := fmt.Sprintf("%s — %s", t.Name, describeMatch(rule, merged, cur, prev, reachable))
	dests := resolveDestinations(cfg, rule)
	if len(dests) == 0 {
		return
	}
	e.lastFired[key] = time.Now()
	for _, d := range dests {
		go func(dest models.NotifyDestination) {
			if err := Send(dest, title, msg); err != nil {
				if e.log != nil {
					e.log.Warnf("notify: %q → %s failed: %v", rule.Name, dest.Name, err)
				}
				return
			}
			if e.log != nil {
				e.log.Infof("notify: %q fired → %s (%s)", rule.Name, dest.Name, t.Name)
			}
		}(d)
	}
}

// resolveDestinations returns the enabled destinations a rule targets (empty
// rule.Destinations = all enabled destinations).
func resolveDestinations(cfg models.NotificationConfig, rule models.AlertRule) []models.NotifyDestination {
	want := map[string]bool{}
	for _, id := range rule.Destinations {
		want[id] = true
	}
	var out []models.NotifyDestination
	for _, d := range cfg.Destinations {
		if !d.Enabled {
			continue
		}
		if len(want) == 0 || want[d.ID] {
			out = append(out, d)
		}
	}
	return out
}

func evalRule(rule models.AlertRule, merged models.MergedStats, cur, prev map[string]string, reachable bool) bool {
	any := rule.Match == "any"
	for _, c := range rule.Conditions {
		ok := evalCondition(c, merged, cur, prev, reachable)
		if any && ok {
			return true
		}
		if !any && !ok {
			return false
		}
	}
	// AND with all true → true; OR with none true → false.
	return !any
}

func evalCondition(c models.Condition, merged models.MergedStats, cur, prev map[string]string, reachable bool) bool {
	switch c.Field {
	case "reachable":
		return boolMatch(c.Op, reachable)
	case "freeleech_active":
		_, active := merged["active_event"]
		return boolMatch(c.Op, active)
	case "unread_mail", "unread_notifications":
		// Scraped presence flags ("true"/"false"; unset = unknown → not true).
		return boolMatch(c.Op, cur[c.Field] == "true")
	}
	if c.Op == "changed" {
		p, hadPrev := prev[c.Field]
		return hadPrev && p != cur[c.Field]
	}
	// Numeric comparison.
	unit := numericFields[c.Field]
	have, ok := numericValue(c.Field, unit, cur[c.Field])
	if !ok {
		return false
	}
	want, ok := numericValue(c.Field, unit, c.Value)
	if !ok {
		return false
	}
	switch c.Op {
	case "lt":
		return have < want
	case "lte":
		return have <= want
	case "gt":
		return have > want
	case "gte":
		return have >= want
	case "eq":
		return have == want
	case "ne":
		return have != want
	}
	return false
}

func boolMatch(op string, v bool) bool {
	switch op {
	case "is_true":
		return v
	case "is_false":
		return !v
	}
	return false
}

// numericValue parses a field/condition string into a comparable number in the
// field's unit (GiB for sizes, days for durations, raw otherwise).
func numericValue(field, unit, raw string) (float64, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, false
	}
	switch unit {
	case "GiB":
		if g := parse.SizeToGiB(raw); g != nil {
			return *g, true
		}
		// Bare number → already GiB.
		return parse.AnyFloat(raw), true
	case "days":
		if s := parse.SeedTimeToSeconds(raw); s != nil {
			return *s / 86400.0, true
		}
		return parse.AnyFloat(raw), true
	default:
		return parse.AnyFloat(raw), true
	}
}

// rawValues flattens merged fields to plain strings (for changed-detection).
func rawValues(merged models.MergedStats) map[string]string {
	out := make(map[string]string, len(merged))
	for k, f := range merged {
		out[k] = fmt.Sprintf("%v", f.Value)
	}
	return out
}

// eventDescription renders the active event banner text plus its end time
// (e.g. "Global Freeleech (ends Jan 5, 2026 3:00 PM UTC)"). Used so event
// alerts carry the real announcement, not just "freeleech active".
func eventDescription(merged models.MergedStats) string {
	text := ""
	if f, ok := merged["active_event"]; ok {
		text = strings.TrimSpace(fmt.Sprintf("%v", f.Value))
	}
	if text == "" {
		text = "event active"
	}
	out := "event: " + text
	if f, ok := merged["active_event_ends_at"]; ok {
		if sec := toUnixSeconds(f.Value); sec > 0 {
			out += " (ends " + time.Unix(sec, 0).Format("Jan 2, 2006 3:04 PM MST") + ")"
		}
	}
	return out
}

// toUnixSeconds coerces a stored value (JSON number, int, or numeric string)
// to a unix-seconds int64. Returns 0 when it can't be parsed.
func toUnixSeconds(v any) int64 {
	switch n := v.(type) {
	case float64:
		return int64(n)
	case int64:
		return n
	case int:
		return int64(n)
	case string:
		if f, err := strconv.ParseFloat(strings.TrimSpace(n), 64); err == nil {
			return int64(f)
		}
	}
	return 0
}

// describeMatch builds a human description of why a rule fired.
func describeMatch(rule models.AlertRule, merged models.MergedStats, cur, prev map[string]string, reachable bool) string {
	parts := make([]string, 0, len(rule.Conditions))
	for _, c := range rule.Conditions {
		parts = append(parts, describeCondition(c, merged, cur, prev))
	}
	return strings.Join(parts, conditionSep(rule))
}

// describeCondition renders one condition's human description. It is the
// single source of wording for BOTH live alert messages (describeMatch) and
// dry-run previews (describeDryRun) — change it here and both stay in sync.
func describeCondition(c models.Condition, merged models.MergedStats, cur, prev map[string]string) string {
	switch c.Field {
	case "reachable":
		if c.Op == "is_false" {
			return "tracker unreachable"
		}
		return "tracker reachable"
	case "freeleech_active":
		if c.Op == "is_true" {
			// Pass the actual banner text + end time through — these events
			// aren't always freeleech (open registration, double-upload, …).
			return eventDescription(merged)
		}
		return "no active event"
	case "unread_mail":
		if c.Op == "is_true" {
			return "unread mail waiting"
		}
		return "no unread mail"
	case "unread_notifications":
		if c.Op == "is_true" {
			return "unread notifications waiting"
		}
		return "no unread notifications"
	}
	if c.Op == "changed" {
		return fmt.Sprintf("%s changed: %s → %s", c.Field, prev[c.Field], cur[c.Field])
	}
	have := cur[c.Field]
	if have == "" {
		have = "—" // no data for this field (e.g. never scraped)
	}
	return fmt.Sprintf("%s %s %s %s", c.Field, have, opSymbol(c.Op), c.Value)
}

// conditionSep is the joiner between condition descriptions for a rule.
func conditionSep(rule models.AlertRule) string {
	if rule.Match == "any" {
		return " or "
	}
	return " and "
}

func opSymbol(op string) string {
	switch op {
	case "lt":
		return "<"
	case "lte":
		return "≤"
	case "gt":
		return ">"
	case "gte":
		return "≥"
	case "eq":
		return "="
	case "ne":
		return "≠"
	}
	return op
}
