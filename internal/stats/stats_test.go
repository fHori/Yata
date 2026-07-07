package stats

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/Yata-Dash/Yata-Dash/internal/store"
)

func TestGrowthRatesFromDailyRollups(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "r.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	e := New(db)

	// 8 daily rollups, uploaded growing 10 GiB/day (1000 → 1070).
	now := time.Now().UTC()
	for i := 7; i >= 0; i-- {
		at := now.AddDate(0, 0, -i)
		_ = db.RecordDaily("t1", at, map[string]float64{
			"uploaded":     1000 + float64(7-i)*10,
			"bonus_points": 50000 + float64(7-i)*500,
			"seed_size":    2000, // flat → no rate
		})
	}
	r := e.GrowthRates("t1")
	if r["uploaded"] < 9.5 || r["uploaded"] > 10.5 {
		t.Errorf("uploaded rate = %v, want ~10 GiB/day", r["uploaded"])
	}
	if r["bonus_points"] < 490 || r["bonus_points"] > 510 {
		t.Errorf("bonus rate = %v, want ~500/day", r["bonus_points"])
	}
	if _, ok := r["seed_size"]; ok {
		t.Errorf("flat seed_size should be omitted, got %v", r["seed_size"])
	}
}

func TestGrowthRatesFallsBackToFineHistory(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "r2.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	e := New(db)

	// Only fine history (no daily rollups yet — a fresh tracker), 6h span.
	now := time.Now().UTC()
	_ = db.AddHistory("t1", now.Add(-6*time.Hour), map[string]float64{"uploaded": 100})
	_ = db.AddHistory("t1", now, map[string]float64{"uploaded": 105}) // +5 GiB in 6h = 20/day

	r := e.GrowthRates("t1")
	if r["uploaded"] < 18 || r["uploaded"] > 22 {
		t.Errorf("fine-history fallback rate = %v, want ~20 GiB/day", r["uploaded"])
	}
}

func TestRateFromPointsGuards(t *testing.T) {
	// Too-short span → no rate.
	pts := []store.HistoryPoint{
		{RecordedAt: 0, Value: 100},
		{RecordedAt: 3600, Value: 200}, // 1h span
	}
	if _, ok := rateFromPoints(pts, 3*3600); ok {
		t.Error("span under threshold should yield no rate")
	}
	// Declining stat → no rate.
	decl := []store.HistoryPoint{
		{RecordedAt: 0, Value: 200},
		{RecordedAt: 86400, Value: 100},
	}
	if _, ok := rateFromPoints(decl, 3600); ok {
		t.Error("declining stat should yield no rate")
	}
}
