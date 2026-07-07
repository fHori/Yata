// pathsync converts the community tracker-pathways dataset
// (github.com/handokota/trackerpathways, MIT) into Yata's
// defs/pathways/routes.json. Run it to refresh the bundled snapshot:
//
//	go run ./tools/pathsync                  # fetch from GitHub
//	go run ./tools/pathsync -local file.json # convert a local copy
//
// The output is pure data — the app maps tracker names to its own defs at
// load time, so this file needs no hand edits.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"time"
)

const upstreamURL = "https://raw.githubusercontent.com/handokota/trackerpathways/main/src/data/trackers.json"

// ── Upstream shapes ──────────────────────────────────────────────────────────

type upstreamRoute struct {
	Days    *float64 `json:"days"`
	Reqs    string   `json:"reqs"`
	Active  string   `json:"active"`
	Updated string   `json:"updated"`
}

type upstream struct {
	RouteInfo         map[string]map[string]upstreamRoute `json:"routeInfo"`
	UnlockInviteClass map[string][]any                    `json:"unlockInviteClass"`
	AbbrList          map[string]string                   `json:"abbrList"`
}

// ── Yata shapes (must match internal/pathways/types.go) ──────────────────

type route struct {
	From    string `json:"from"`
	To      string `json:"to"`
	Days    int    `json:"days"`            // min account age on source (-1 = unknown)
	Reqs    string `json:"reqs"`            // free-text requirements from the community data
	Active  bool   `json:"active"`
	Updated string `json:"updated,omitempty"`
}

type unlockClass struct {
	Days int    `json:"days"` // days until invites unlock (-1 = unknown)
	Text string `json:"text"` // "Class: req, req; Class2: ..." free text
}

type output struct {
	SchemaVersion int    `json:"schema_version"`
	Source        struct {
		Name    string `json:"name"`
		URL     string `json:"url"`
		License string `json:"license"`
		Fetched string `json:"fetched"`
	} `json:"source"`
	Abbr    map[string]string      `json:"abbr,omitempty"`
	Routes  []route                `json:"routes"`
	Unlocks map[string]unlockClass `json:"unlocks"`
}

func main() {
	local := flag.String("local", "", "convert a local trackers.json instead of fetching")
	out := flag.String("out", filepath.Join("defs", "pathways", "routes.json"), "output path")
	flag.Parse()

	var raw []byte
	var err error
	if *local != "" {
		raw, err = os.ReadFile(*local)
	} else {
		raw, err = fetch(upstreamURL)
	}
	if err != nil {
		log.Fatal(err)
	}

	var up upstream
	if err := json.Unmarshal(raw, &up); err != nil {
		log.Fatalf("parse upstream: %v", err)
	}

	var o output
	o.SchemaVersion = 1
	o.Source.Name = "trackerpathways"
	o.Source.URL = "https://github.com/handokota/trackerpathways"
	o.Source.License = "MIT"
	o.Source.Fetched = time.Now().UTC().Format("2006-01-02")
	o.Abbr = up.AbbrList
	o.Unlocks = map[string]unlockClass{}

	for src, targets := range up.RouteInfo {
		for dst, r := range targets {
			days := -1
			if r.Days != nil {
				days = int(*r.Days)
			}
			o.Routes = append(o.Routes, route{
				From:    src,
				To:      dst,
				Days:    days,
				Reqs:    r.Reqs,
				Active:  r.Active == "Yes",
				Updated: r.Updated,
			})
		}
	}
	sort.Slice(o.Routes, func(i, j int) bool {
		if o.Routes[i].From != o.Routes[j].From {
			return o.Routes[i].From < o.Routes[j].From
		}
		return o.Routes[i].To < o.Routes[j].To
	})

	for name, uc := range up.UnlockInviteClass {
		entry := unlockClass{Days: -1}
		if len(uc) > 0 {
			if f, ok := uc[0].(float64); ok {
				entry.Days = int(f)
			}
		}
		if len(uc) > 1 {
			if s, ok := uc[1].(string); ok {
				entry.Text = s
			}
		}
		o.Unlocks[name] = entry
	}

	data, err := json.MarshalIndent(o, "", "  ")
	if err != nil {
		log.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(*out), 0o755); err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile(*out, append(data, '\n'), 0o644); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("wrote %s: %d routes, %d unlock entries, fetched %s\n",
		*out, len(o.Routes), len(o.Unlocks), o.Source.Fetched)
}

func fetch(url string) ([]byte, error) {
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("http %d fetching %s", resp.StatusCode, url)
	}
	return io.ReadAll(resp.Body)
}
