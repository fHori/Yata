// demoseed builds a complete DEMO instance for screenshots/docs: real tracker
// defs, a dummy user, and varied synthetic stats. It REPLACES the trackers,
// notifications, and relevant settings of the target config and RECREATES the
// database. Point it at a scratch config/db — never at your real instance.
//
//	go run ./tools/demoseed -config path\to\demo.json -db path\to\demo.db
//
// No credentials are written, so the app never contacts the real trackers
// (API fetches fail pre-flight with no_key; scrapes are blocked by no_cookie).
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/Yata-Dash/Yata-Dash/internal/models"
	"github.com/Yata-Dash/Yata-Dash/internal/stats"
	"github.com/Yata-Dash/Yata-Dash/internal/store"
)

const user = "DemoUser"

type demo struct {
	t       models.Tracker
	api     map[string]any // API stat layer
	scr     map[string]any // scrape stat layer (shows provenance dots)
	upGiB   float64        // current uploaded, GiB (history baseline)
	upRate  float64        // GiB/day growth
	ssGiB   float64        // current seed size, GiB
	ssRate  float64
	bonus   float64
	bonRate float64
	bufGiB  float64
	bufRate float64 // may be negative
}

func main() {
	cfgPath := flag.String("config", "", "config.json to REPLACE demo sections in")
	dbPath := flag.String("db", "", "SQLite database path (recreated)")
	flag.Parse()
	if *cfgPath == "" || *dbPath == "" {
		log.Fatal("-config and -db are required")
	}

	now := time.Now()
	flEnds := now.Add(50 * time.Hour).Unix()

	demos := []demo{
		{
			t: models.Tracker{
				ID: "demoseedpool0001", Name: "seedpool", URL: "https://seedpool.org", Type: "unit3d",
				Enabled: true, Username: user, JoinDate: "2024-06-15", TargetGroup: "SuperPool",
				Targets: map[string]string{"ratio": "1", "days": "183", "seed_size": "1 TiB"},
			},
			api: map[string]any{
				"username": user, "group": "PowerPool", "uploaded": "8.60 TiB", "downloaded": "2.10 TiB",
				"buffer": "6.50 TiB", "ratio": 4.10, "seeding": 640, "leeching": 1, "hit_and_runs": 0,
				"bonus_points": "126400", "join_date": "2024-06-15",
			},
			scr: map[string]any{"seed_size": "870.00 GiB", "avg_seed_time": "3M 2W", "fl_tokens": "6", "warnings": "0",
				"unread_mail": "true", "unread_notifications": "false"},
			upGiB: 8806, upRate: 35, ssGiB: 870, ssRate: 11, bonus: 126400, bonRate: 3400, bufGiB: 6656, bufRate: 28,
		},
		{
			t: models.Tracker{
				ID: "demoaither000001", Name: "Aither", URL: "https://aither.cc", Type: "unit3d",
				Enabled: true, Username: user, JoinDate: "2024-11-08", TargetGroup: "Helios",
				Targets: map[string]string{"ratio": "0.8", "days": "183", "avg_seed": "1728000", "total_uploads": "1"},
			},
			api: map[string]any{
				"username": user, "group": "Zeus", "uploaded": "12.40 TiB", "downloaded": "3.20 TiB",
				"buffer": "9.20 TiB", "ratio": 3.88, "seeding": 412, "leeching": 2, "hit_and_runs": 0,
				"bonus_points": "84250", "uploads_approved": "6", "join_date": "2024-11-08",
				"active_event": "Global Freeleech", "active_event_ends_at": flEnds,
			},
			scr: map[string]any{"seed_size": "6.90 TiB", "avg_seed_time": "2M 1W", "fl_tokens": "8", "warnings": "0",
				"unread_mail": "true", "unread_notifications": "true"},
			upGiB: 12697, upRate: 42, ssGiB: 7066, ssRate: 15, bonus: 84250, bonRate: 2900, bufGiB: 9421, bufRate: 31,
		},
		{
			t: models.Tracker{
				ID: "demolst000000001", Name: "LST", URL: "https://lst.gg", Type: "unit3d",
				Enabled: true, Username: user, JoinDate: "2025-02-14", TargetGroup: "Dolphin",
				Targets: map[string]string{"total_uploads": "5", "ratio": "1", "avg_seed": "1728000", "days": "91"},
			},
			api: map[string]any{
				"username": user, "group": "Goldfish", "uploaded": "3.60 TiB", "downloaded": "1.10 TiB",
				"buffer": "2.50 TiB", "ratio": 3.27, "seeding": 188, "leeching": 1, "hit_and_runs": 0,
				"bonus_points": "15400", "join_date": "2025-02-14", "uploads_approved": "3",
			},
			scr:   map[string]any{"seed_size": "1.35 TiB", "avg_seed_time": "3W 2D", "fl_tokens": "3"},
			upGiB: 3686, upRate: 18, ssGiB: 1382, ssRate: 9, bonus: 15400, bonRate: 950, bufGiB: 2560, bufRate: 12,
		},
		{
			t: models.Tracker{
				ID: "demoant000000001", Name: "Anthelion", URL: "https://anthelion.me", Type: "gazelle",
				Enabled: true, Username: user, JoinDate: "2025-06-20", TargetGroup: "Power User",
				Targets: map[string]string{"uploaded": "1 TiB", "ratio": "1", "bonus_points": "25000", "days": "30"},
			},
			api: map[string]any{
				"username": user, "group": "Member", "uploaded": "1.40 TiB", "downloaded": "900.00 GiB",
				"buffer": "533.60 GiB", "ratio": 1.59, "seeding": 96, "leeching": 0,
				"invites": "2", "snatched": "156", "join_date": "2025-06-20",
			},
			scr: map[string]any{
				"bonus_points": "31200", "uploads_approved": "2", "adoptions": "4",
				"fl_tokens": "12", "hit_and_runs": "0",
			},
			upGiB: 1433, upRate: 11, ssGiB: 0, ssRate: 0, bonus: 31200, bonRate: 620, bufGiB: 533, bufRate: 6,
		},
		{
			t: models.Tracker{
				ID: "demozenith000001", Name: "Zenith", URL: "https://znth.cx", Type: "unit3d",
				Enabled: true, Username: user, JoinDate: "2025-05-02", TargetGroup: "Bulker",
				Targets: map[string]string{"ratio": "0.69", "days": "21", "avg_seed": "604800"},
			},
			api: map[string]any{
				"username": user, "group": "Seeker", "uploaded": "2.20 TiB", "downloaded": "800.00 GiB",
				"buffer": "1.42 TiB", "ratio": 2.82, "seeding": 143, "leeching": 3, "hit_and_runs": 0,
				"bonus_points": "9800", "join_date": "2025-05-02",
			},
			scr:   map[string]any{"seed_size": "3.10 TiB", "avg_seed_time": "1M 1W"},
			upGiB: 2252, upRate: 22, ssGiB: 3174, ssRate: 20, bonus: 9800, bonRate: 410, bufGiB: 1454, bufRate: 14,
		},
		{
			t: models.Tracker{
				ID: "demomam000000001", Name: "MyAnonamouse", URL: "https://www.myanonamouse.net", Type: "custom",
				Enabled: true, Username: user, JoinDate: "2023-09-01",
				Targets: map[string]string{"uploaded": "10 TiB", "ratio": "2"},
			},
			api: map[string]any{
				"username": user, "group": "Power User", "uploaded": "5.10 TiB", "downloaded": "1.20 TiB",
				"buffer": "3.90 TiB", "ratio": 4.25, "seeding": 350, "leeching": 0,
				"bonus_points": "152430", "join_date": "2023-09-01",
				"seed_size": "4.20 TiB",
			},
			upGiB: 5222, upRate: 25, ssGiB: 4300, ssRate: 18, bonus: 152430, bonRate: 4100, bufGiB: 3994, bufRate: 22,
		},
	}

	// ── Config: keep server section, replace everything demo-relevant ──────
	cfg := models.Config{Server: models.ServerConfig{Host: "127.0.0.1", Port: 8420}}
	if raw, err := os.ReadFile(*cfgPath); err == nil {
		var existing models.Config
		if json.Unmarshal(raw, &existing) == nil && existing.Server.Port != 0 {
			cfg.Server = existing.Server
		}
	}
	cfg.Settings = models.DefaultSettings()
	cfg.Settings.ShowStatSources = true
	cfg.Settings.ProfileAutoSync = false
	for _, dm := range demos {
		cfg.Trackers = append(cfg.Trackers, dm.t)
	}
	cfg.Notifications = models.NotificationConfig{
		Destinations: []models.NotifyDestination{
			{ID: "demodest00000001", Name: "Team Discord", Type: "discord",
				URL: "https://discord.com/api/webhooks/000000/DEMO", Enabled: false},
			{ID: "demodest00000002", Name: "Gotify", Type: "gotify",
				URL: "https://gotify.example", Token: "DEMO-TOKEN", Enabled: false},
		},
		Rules: []models.AlertRule{
			{ID: "demorule00000001", Name: "Low ratio guard", Enabled: true,
				TrackerMode: "include", Match: "all", CooldownMins: 360,
				Conditions:   []models.Condition{{Field: "ratio", Op: "lt", Value: "1.0"}},
				Destinations: []string{"demodest00000001"}},
			{ID: "demorule00000002", Name: "Freeleech spotter", Enabled: true,
				TrackerMode: "include", Match: "any", CooldownMins: 720,
				TrackerIDs:  []string{"demoaither000001", "demolst000000001"},
				Conditions:  []models.Condition{{Field: "freeleech_active", Op: "is_true"}}},
		},
	}
	out, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile(*cfgPath, out, 0o644); err != nil {
		log.Fatal(err)
	}

	// ── Database: recreate from scratch ─────────────────────────────────────
	for _, suffix := range []string{"", "-wal", "-shm"} {
		_ = os.Remove(*dbPath + suffix)
	}
	db, err := store.Open(*dbPath)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	eng := stats.New(db)

	for _, dm := range demos {
		if err := eng.SaveAPI(dm.t.ID, dm.api); err != nil {
			log.Fatal(err)
		}
		if len(dm.scr) > 0 {
			if err := eng.SaveScrape(dm.t.ID, dm.scr); err != nil {
				log.Fatal(err)
			}
		}
		// Daily rollups (trend rates + target ETAs): 10 days of growth.
		for i := 10; i >= 0; i-- {
			at := now.Add(-time.Duration(i) * 24 * time.Hour)
			f := map[string]float64{
				"uploaded":     dm.upGiB - float64(i)*dm.upRate,
				"bonus_points": dm.bonus - float64(i)*dm.bonRate,
				"buffer":       dm.bufGiB - float64(i)*dm.bufRate,
			}
			if dm.ssGiB > 0 {
				f["seed_size"] = dm.ssGiB - float64(i)*dm.ssRate
			}
			if err := db.RecordDaily(dm.t.ID, at, f); err != nil {
				log.Fatal(err)
			}
		}
		// Fine history (48h sparklines): a point every 3 hours.
		for i := 16; i >= 0; i-- {
			at := now.Add(-time.Duration(i) * 3 * time.Hour)
			frac := float64(i) / 8.0 // 2 days' worth of the daily rate
			f := map[string]float64{
				"uploaded":     dm.upGiB - frac*dm.upRate,
				"bonus_points": dm.bonus - frac*dm.bonRate,
				"buffer":       dm.bufGiB - frac*dm.bufRate,
			}
			if dm.ssGiB > 0 {
				f["seed_size"] = dm.ssGiB - frac*dm.ssRate
			}
			if err := db.AddHistory(dm.t.ID, at, f); err != nil {
				log.Fatal(err)
			}
		}
	}
	fmt.Printf("demo instance ready: %d trackers → %s / %s\n", len(demos), *cfgPath, *dbPath)
}
