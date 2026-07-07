package api

import (
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/Yata-Dash/Yata-Dash/internal/models"
	"github.com/Yata-Dash/Yata-Dash/internal/scrape"
)

func registerStats(r chi.Router, d *Deps) {
	r.Get("/stats", bulkStats(d))
	r.Get("/stats/{id}", singleStats(d))
}

// refreshTracker fetches fresh API data for one tracker, persists it to the
// API layer, and returns the merged view. On API failure it attempts a
// profile-scrape fallback for the main stats — but only when the scrape
// policy allows it (v2 fix: the fallback respects rate limits and disable
// flags, and is logged in scrape_log like any other scrape).
//
// IMPORTANT (v1 lesson): this runs inside goroutines for bulk refresh.
// The config manager is mutex-safe; never add file reloads here.
func refreshTracker(d *Deps, t models.Tracker) models.TrackerStatsResponse {
	resp := models.TrackerStatsResponse{
		TrackerID: t.ID,
		FetchedAt: time.Now().Unix(),
	}

	data, ferr := d.Fetch.Fetch(t)
	if ferr == nil {
		_ = d.Stats.SaveAPI(t.ID, data)
		resp.OK = true
		logFetchTransition(d, t, "")

		// Auto-save username/join date the first time the API reveals them.
		if u, ok := data["username"].(string); ok && u != "" && t.Username == "" {
			_ = d.Cfg.UpdateTracker(t.ID, func(tr *models.Tracker) { tr.Username = u })
		}
	} else {
		resp.ErrorKind = ferr.Kind
		resp.Error = ferr.Error()
		logFetchTransition(d, t, ferr.Kind)
		// API failed — try a policy-respecting scrape fallback for main stats.
		tryScrapeFallback(d, t)
	}

	merged, err := d.Stats.Merged(t.ID)
	if err == nil {
		resp.Fields = merged
		if resp.OK {
			_ = d.Stats.RecordHistory(t.ID, merged)
		}
		// Growth rates (daily-rollup average) for target/promotion ETAs.
		if r := d.Stats.GrowthRates(t.ID); len(r) > 0 {
			resp.Rates = r
		}
	} else if resp.Error == "" {
		resp.OK = false
		resp.Error = "store_error"
	}

	// Alert evaluation — fires webhooks on rising-edge rule matches. Runs on
	// every refresh path (frontend poll, single, or the background loop);
	// edge-triggering + per-tracker priming keep it from spamming.
	if d.Alerts != nil {
		d.Alerts.Evaluate(t, resp.Fields, resp.OK)
	}
	return resp
}

// lastFetchState remembers each tracker's previous API-fetch outcome so the
// refresh loop logs TRANSITIONS (ok→fail at warn, fail→ok at info) instead of
// re-warning on every cycle — a tracker that stays down would otherwise flood
// the log once per refresh. Keyed by tracker ID; value = last error kind
// ("" = ok). Repeat failures of the same kind log at debug.
var lastFetchState sync.Map

func logFetchTransition(d *Deps, t models.Tracker, errKind string) {
	prev, seen := lastFetchState.Load(t.ID)
	lastFetchState.Store(t.ID, errKind)
	switch {
	case errKind == "" && seen && prev != "":
		d.logInfof("fetch: %s (%s) recovered — API reachable again", t.Name, t.ID)
	case errKind != "" && (!seen || prev == ""):
		d.logWarnf("fetch: %s (%s) failed — %s", t.Name, t.ID, errKind)
	case errKind != "" && prev != errKind:
		d.logWarnf("fetch: %s (%s) still failing — now %s (was %s)", t.Name, t.ID, errKind, prev)
	case errKind != "":
		d.logDebugf("fetch: %s (%s) still failing — %s", t.Name, t.ID, errKind)
	}
}

// scrapeLocks serialises all scrape activity per tracker. Every path that
// can trigger an HTTP request to a tracker's profile page (manual scrape
// endpoint, auto-sync, API-failure fallback — possibly from multiple browser
// tabs at once) must hold the tracker's lock across the ENTIRE
// evaluate→scrape→record sequence. Without it, two concurrent requests could
// both pass the policy check before either records, double-hitting the
// tracker. Rate limits protect users' accounts — they must be airtight.
var scrapeLocks sync.Map // trackerID → *sync.Mutex

func lockScrape(trackerID string) *sync.Mutex {
	m, _ := scrapeLocks.LoadOrStore(trackerID, &sync.Mutex{})
	mu := m.(*sync.Mutex)
	mu.Lock()
	return mu
}

// tryScrapeFallback scrapes the profile page when the API is down, writing
// to the scrape layer. Unlike v1, it goes through the full policy check so a
// dead API can never cause scrape-hammering of a tracker.
func tryScrapeFallback(d *Deps, t models.Tracker) {
	if strings.TrimSpace(t.Username) == "" {
		return
	}
	mu := lockScrape(t.ID)
	defer mu.Unlock()

	// Policy MUST be evaluated inside the lock — a concurrent scrape may have
	// just recorded an attempt that puts us in cooldown.
	rs := d.Reg.ResolveScrape(t.URL, t.Type)
	pol := scrape.Evaluate(d.Cfg.Settings(), t, rs, d.DB, time.Now())
	if !pol.Allowed {
		return
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
	if serr != nil || len(result) == 0 {
		return
	}
	_ = d.Stats.SaveScrape(t.ID, toAnyMap(result))
}

// recordScrapeAttempt logs a scrape in the rate-limit ledger whenever an HTTP
// request actually reached the tracker — including failed ones. A profile
// page that errors must not get re-hit on every refresh cycle; only
// pre-flight failures (no username/key — nothing was sent) are exempt.
func recordScrapeAttempt(d *Deps, trackerID string, serr *scrape.Error) {
	if serr != nil && (serr.Kind == "no_username" || serr.Kind == "no_cookie" || serr.Kind == "no_key") {
		return // pre-flight failure — no request reached the tracker
	}
	_ = d.DB.RecordScrape(trackerID, time.Now().UTC())
}

func toAnyMap(in map[string]string) map[string]any {
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

// RunRefreshCycle refreshes every enabled tracker once. It's used by the
// server-side scheduler so stats stay fresh and alert rules are evaluated even
// when no browser/homelab client is polling /api/stats. Sequential by design
// (gentle on tracker APIs).
func RunRefreshCycle(d *Deps) {
	for _, t := range d.Cfg.Trackers() {
		if !t.Enabled {
			continue
		}
		_ = refreshTracker(d, t)
	}
}

// GET /api/stats — refresh all enabled trackers concurrently.
func bulkStats(d *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		trackers := d.Cfg.Trackers()
		results := make(map[string]models.TrackerStatsResponse, len(trackers))
		var mu sync.Mutex
		var wg sync.WaitGroup
		for _, t := range trackers {
			if !t.Enabled {
				mu.Lock()
				results[t.ID] = models.TrackerStatsResponse{TrackerID: t.ID, ErrorKind: "disabled", Error: "disabled"}
				mu.Unlock()
				continue
			}
			wg.Add(1)
			go func(t models.Tracker) {
				defer wg.Done()
				res := refreshTracker(d, t)
				mu.Lock()
				results[t.ID] = res
				mu.Unlock()
			}(t)
		}
		wg.Wait()
		jsonOK(w, results)
	}
}

// GET /api/stats/{id} — refresh one tracker.
func singleStats(d *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		t, ok := d.Cfg.Tracker(id)
		if !ok {
			jsonError(w, "tracker not found", http.StatusNotFound)
			return
		}
		if !t.Enabled {
			jsonOK(w, models.TrackerStatsResponse{TrackerID: t.ID, ErrorKind: "disabled", Error: "disabled"})
			return
		}
		jsonOK(w, refreshTracker(d, t))
	}
}
