package api

import (
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/Yata-Dash/Yata-Dash/internal/models"
	"github.com/Yata-Dash/Yata-Dash/internal/scrape"
)

// CheckResult is the outcome of one connectivity check (API or scrape).
//
//	ok             — the request succeeded
//	fail           — a request was made but failed (Detail = error kind)
//	not_configured — a required credential is missing (Detail = which one)
//	not_applicable — this tracker doesn't use this method (Detail = why)
//	blocked        — testing now would break the scrape rate limits
//	                 (Detail = cooldown | daily_limit); try again later
type CheckResult struct {
	Status string `json:"status"`
	Detail string `json:"detail,omitempty"`
	Fields int    `json:"fields,omitempty"`
}

// TrackerTestResult is the combined API + scrape connectivity test for one
// tracker, so the user can see exactly which of the two (or both) work.
type TrackerTestResult struct {
	API      CheckResult `json:"api"`
	Scrape   CheckResult `json:"scrape"`
	TestedAt int64       `json:"tested_at"` // unix seconds
}

// testResults caches the last test outcome per tracker so the trackers table
// can show a status indicator without re-hitting the tracker on every render.
// Cleared lazily — a deleted tracker's stale entry is harmless.
var testResults sync.Map // trackerID → TrackerTestResult

// POST /api/trackers/{id}/test — actively test the tracker's API and profile
// scrape and return which works. Caches the result for the table indicator.
func testTracker(d *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		t, ok := d.Cfg.Tracker(id)
		if !ok {
			jsonError(w, "tracker not found", http.StatusNotFound)
			return
		}
		res := runTrackerTest(d, t)
		testResults.Store(t.ID, res)
		// Level follows the outcome: a user asked for this test, so a failed
		// or rate-limit-blocked check is a warning, not a routine info line.
		msg := fmt.Sprintf("test: %s (%s) — api=%s scrape=%s",
			t.Name, t.ID, fmtCheck(res.API), fmtCheck(res.Scrape))
		if res.API.Status == "fail" || res.Scrape.Status == "fail" ||
			res.Scrape.Status == "blocked" {
			d.logWarnf("%s", msg)
		} else {
			d.logInfof("%s", msg)
		}
		jsonOK(w, res)
	}
}

// GET /api/trackers/test-status — cached last-test results for every tracker
// (the trackers table reads this on load; absent entries = "not tested yet").
func testStatusAll(d *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		out := map[string]TrackerTestResult{}
		for _, t := range d.Cfg.Trackers() {
			if v, ok := testResults.Load(t.ID); ok {
				out[t.ID] = v.(TrackerTestResult)
			}
		}
		jsonOK(w, out)
	}
}

// fmtCheck renders one check outcome for the log line, keeping the detail
// ("fail" alone hides WHY — connection_error vs rate limit vs bad key).
func fmtCheck(c CheckResult) string {
	if c.Detail == "" {
		return c.Status
	}
	return c.Status + ":" + c.Detail
}

// runTrackerTest runs both checks. On success it persists the freshly fetched
// data (a test doubles as a refresh) — the scrape, being a real request, also
// records a rate-limit attempt just like a normal scrape.
func runTrackerTest(d *Deps, t models.Tracker) TrackerTestResult {
	return TrackerTestResult{
		API:      testAPI(d, t),
		Scrape:   testScrape(d, t),
		TestedAt: time.Now().Unix(),
	}
}

func testAPI(d *Deps, t models.Tracker) CheckResult {
	kind := d.Reg.APIKind(t.URL, t.Type)
	if kind == "none" {
		return CheckResult{Status: "not_applicable", Detail: "scrape_only"}
	}
	// Real APIs need a key (and gazelle also a username) — surface these as
	// "not configured" rather than letting the fetcher return a raw error.
	if kind != "demo" {
		if strings.TrimSpace(t.APIKey) == "" {
			return CheckResult{Status: "not_configured", Detail: "no_key"}
		}
		if kind == "gazelle" && strings.TrimSpace(t.Username) == "" {
			return CheckResult{Status: "not_configured", Detail: "no_username"}
		}
	}
	fields, ferr := d.Fetch.Fetch(t)
	if ferr != nil {
		return CheckResult{Status: "fail", Detail: ferr.Kind}
	}
	_ = d.Stats.SaveAPI(t.ID, fields)
	return CheckResult{Status: "ok", Fields: len(fields)}
}

func testScrape(d *Deps, t models.Tracker) CheckResult {
	// Demo trackers never scrape.
	if t.Type == "test" {
		return CheckResult{Status: "not_applicable", Detail: "no_scrape_support"}
	}

	// Hold the per-tracker lock across evaluate→scrape→record (same contract
	// as runScrape) so a test can never race a refresh into double-hitting the
	// tracker — and evaluate the SAME policy cascade. A test that bypassed
	// cooldowns or daily caps would let the Test button hammer a tracker;
	// rate limits protect users' accounts and must stay airtight.
	mu := lockScrape(t.ID)
	defer mu.Unlock()

	rs := d.Reg.ResolveScrape(t.URL, t.Type)
	pol := scrape.Evaluate(d.Cfg.Settings(), t, rs, d.DB, time.Now())
	if !pol.Allowed {
		switch pol.Reason {
		case "api_only", "no_scrape_support", "scrape_disabled":
			return CheckResult{Status: "not_applicable", Detail: pol.Reason}
		case "no_username", "no_cookie":
			return CheckResult{Status: "not_configured", Detail: pol.Reason}
		default: // cooldown | daily_limit — a request now would break the limits
			return CheckResult{Status: "blocked", Detail: pol.Reason}
		}
	}

	spec := scrape.Spec{
		ExtraLabels:     rs.Labels,
		ProfilePath:     rs.ProfilePath,
		EventTitleClass: rs.EventTitleClass,
		StatCardClasses: rs.StatCardClasses,
		PresenceFlags:   rs.PresenceFlags,
		Identify:        rs.Identify,
		Gazelle:         d.Reg.APIKind(t.URL, t.Type) == "gazelle",
		KnownUserID:     mergedString(d, t.ID, "user_id"),
	}
	result, serr := scrape.Profile(t, spec)
	recordScrapeAttempt(d, t.ID, serr)
	if serr != nil {
		return CheckResult{Status: "fail", Detail: serr.Kind}
	}
	if len(result) > 0 {
		_ = d.Stats.SaveScrape(t.ID, toAnyMap(result))
	}
	return CheckResult{Status: "ok", Fields: len(result)}
}
