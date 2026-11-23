package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestNewTuneWritesOneCard: 保存 with no card lit starts tune-1, and 更新 writes
// the numbers being heard back into that same card instead of making a second
// one — a player who keeps adjusting keeps one card, and 另存 is the way to make
// another.
func TestNewTuneWritesOneCard(t *testing.T) {
	root := t.TempDir()
	a := &App{root: root, cfg: config{FilterLevel: "normal", CRTTune: map[string]float32{"scan": 0.4}}}
	a.tuneIdx = -1

	if err := a.newTune(); err != nil {
		t.Fatal(err)
	}
	if a.cfg.FilterTune != "tune-1.json" || len(a.tuneFiles) != 1 || a.tuneIdx != 0 {
		t.Fatalf("saved as %q with %v idx %d, want one card tune-1.json lit", a.cfg.FilterTune, a.tuneFiles, a.tuneIdx)
	}

	a.cfg.CRTTune["scan"] = 0.8
	if err := a.updateTune(); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(a.tuneDir())
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("updating left %d files, want 1", len(entries))
	}
	data, err := os.ReadFile(filepath.Join(a.tuneDir(), "tune-1.json"))
	if err != nil {
		t.Fatal(err)
	}
	var p tunePreset
	if err := json.Unmarshal(data, &p); err != nil {
		t.Fatal(err)
	}
	if p.Level != "normal" || p.Tune["scan"] != 0.8 {
		t.Fatalf("saved group = %+v, want normal with scan 0.8", p)
	}

	// 另存 puts a second card beside the first and lights it.
	if err := a.newTune(); err != nil {
		t.Fatal(err)
	}
	if len(a.tuneFiles) != 2 || a.tuneIdx != 1 || a.cfg.FilterTune != "tune-2.json" {
		t.Fatalf("另存 left %v idx %d as %q, want two cards with tune-2 lit",
			a.tuneFiles, a.tuneIdx, a.cfg.FilterTune)
	}
}

// TestSelectAndDeleteTune: a card brings back its whole look — preset and
// numbers — and 删除 takes the card away and lights the one that slides into its
// place, with 保存 coming back once the last card is gone.
func TestSelectAndDeleteTune(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "数据", "filters")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	one := tunePreset{Level: "normal", Tune: map[string]float32{"scan": 0.2}}
	two := tunePreset{Level: "strong", Tune: map[string]float32{"scan": 0.9}}
	for name, p := range map[string]tunePreset{"tune-1.json": one, "tune-2.json": two} {
		data, err := json.Marshal(p)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	a := &App{root: root, cfg: config{FilterTune: "tune-1.json", FilterLevel: "normal"}}
	a.scanTunes()
	if len(a.tuneFiles) != 2 || a.tuneIdx != 0 {
		t.Fatalf("tunes = %v idx = %d, want two with tune-1 lit", a.tuneFiles, a.tuneIdx)
	}

	if err := a.selectTune(1); err != nil {
		t.Fatal(err)
	}
	if a.cfg.FilterTune != "tune-2.json" || a.cfg.FilterLevel != "strong" || a.cfg.CRTTune["scan"] != 0.9 {
		t.Fatalf("select = %q %q %v, want tune-2 strong 0.9",
			a.cfg.FilterTune, a.cfg.FilterLevel, a.cfg.CRTTune)
	}

	if err := a.deleteTune(); err != nil {
		t.Fatal(err)
	}
	if len(a.tuneFiles) != 1 || a.tuneIdx != 0 {
		t.Fatalf("after delete tunes = %v idx = %d, want tune-1 lit", a.tuneFiles, a.tuneIdx)
	}
	if a.cfg.FilterTune != "tune-1.json" || a.cfg.FilterLevel != "normal" || a.cfg.CRTTune["scan"] != 0.2 {
		t.Fatalf("after delete = %q %q %v, want tune-1 normal 0.2",
			a.cfg.FilterTune, a.cfg.FilterLevel, a.cfg.CRTTune)
	}

	if err := a.deleteTune(); err != nil {
		t.Fatal(err)
	}
	if len(a.tuneFiles) != 0 || a.tuneIdx != -1 || a.cfg.FilterTune != "" {
		t.Fatalf("last card left %v idx %d as %q, want none", a.tuneFiles, a.tuneIdx, a.cfg.FilterTune)
	}
	if a.cfg.CRTTune["scan"] != 0.2 {
		t.Fatalf("deleting the last card dropped the numbers: %v", a.cfg.CRTTune)
	}
}

// TestSetFilterOnKeepsTheTune: the switch off then on is a look change, not a
// reset: the numbers the player moved are still there.
func TestSetFilterOnKeepsTheTune(t *testing.T) {
	a := &App{root: t.TempDir(), cfg: config{FilterLevel: "off", CRTTune: map[string]float32{"scan": 0.6}}}
	a.setFilterOn(true)
	if a.cfg.FilterLevel != "normal" {
		t.Fatalf("on = %q, want normal", a.cfg.FilterLevel)
	}
	if a.cfg.CRTTune["scan"] != 0.6 {
		t.Fatalf("on dropped the tune: %v", a.cfg.CRTTune)
	}
	a.setFilterOn(false)
	if a.cfg.FilterLevel != "off" || a.cfg.CRTTune["scan"] != 0.6 {
		t.Fatalf("off = %q %v, want off with the tune kept", a.cfg.FilterLevel, a.cfg.CRTTune)
	}
}
