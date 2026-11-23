package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// tunePreset is one saved group under 数据/filters: the look it was saved at,
// and the numbers the player had moved. A group is a whole look, so switching
// to it brings back both the preset and the sliders.
type tunePreset struct {
	Level string             `json:"level,omitempty"`
	Tune  map[string]float32 `json:"tune"`
}

// tuneDir is where the saved groups live.
func (a *App) tuneDir() string { return filepath.Join(a.root, "数据", "filters") }

// tuneNumber is the n of tune-<n>.json, or 0 when the name is not one of ours.
func tuneNumber(name string) int {
	if !strings.HasPrefix(name, "tune-") || !strings.HasSuffix(name, ".json") {
		return 0
	}
	n, err := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(name, "tune-"), ".json"))
	if err != nil || n <= 0 {
		return 0
	}
	return n
}

// scanTunes lists the saved groups in playing order and finds the one the
// config says was last heard. It is called at startup and whenever the panel
// opens, so a file dropped in by hand shows up without a restart.
func (a *App) scanTunes() {
	a.tuneFiles = nil
	a.tuneIdx = -1
	if a.root == "" {
		return
	}
	entries, err := os.ReadDir(a.tuneDir())
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() || tuneNumber(e.Name()) == 0 {
			continue
		}
		a.tuneFiles = append(a.tuneFiles, e.Name())
	}
	sort.Slice(a.tuneFiles, func(i, j int) bool {
		return tuneNumber(a.tuneFiles[i]) < tuneNumber(a.tuneFiles[j])
	})
	if a.cfg.FilterTune != "" {
		for i, name := range a.tuneFiles {
			if name == a.cfg.FilterTune {
				a.tuneIdx = i
				break
			}
		}
	}
}

// saveCurrentTune writes the numbers the player has now into the group they
// are hearing; when none has been picked it starts a new tune-<n>.json after
// the last one. Saving twice in a row overwrites the same group instead of
// piling up copies.
func (a *App) saveCurrentTune() error {
	if a.root == "" {
		return errors.New("没有数据目录")
	}
	if err := os.MkdirAll(a.tuneDir(), 0o755); err != nil {
		return err
	}
	if a.tuneIdx < 0 || a.tuneIdx >= len(a.tuneFiles) {
		max := 0
		for _, name := range a.tuneFiles {
			if n := tuneNumber(name); n > max {
				max = n
			}
		}
		name := fmt.Sprintf("tune-%d.json", max+1)
		a.tuneFiles = append(a.tuneFiles, name)
		a.tuneIdx = len(a.tuneFiles) - 1
	}
	name := a.tuneFiles[a.tuneIdx]
	tune := a.cfg.CRTTune
	if tune == nil {
		tune = map[string]float32{}
	}
	data, err := json.MarshalIndent(tunePreset{Level: a.cfg.FilterLevel, Tune: tune}, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(a.tuneDir(), name), append(data, '\n'), 0o644); err != nil {
		return err
	}
	a.cfg.FilterTune = name
	a.saveConfig()
	a.tuneNote = "已保存 " + name
	return nil
}

// cycleTune moves to the next saved group, wrapping at the end, and takes both
// its preset and its numbers. With nothing saved it says so rather than
// pretending to switch.
func (a *App) cycleTune() error {
	if len(a.tuneFiles) == 0 {
		return errors.New("还没有保存的组")
	}
	idx := a.tuneIdx
	if idx < 0 || idx >= len(a.tuneFiles) {
		idx = 0
	} else {
		idx = (idx + 1) % len(a.tuneFiles)
	}
	name := a.tuneFiles[idx]
	data, err := os.ReadFile(filepath.Join(a.tuneDir(), name))
	if err != nil {
		return err
	}
	var p tunePreset
	if err := json.Unmarshal(data, &p); err != nil {
		return err
	}
	if p.Tune == nil {
		p.Tune = map[string]float32{}
	}
	a.cfg.FilterLevel = p.Level
	a.cfg.CRTTune = p.Tune
	a.cfg.FilterTune = name
	a.tuneIdx = idx
	a.dragKey = ""
	a.saveConfig()
	a.tuneNote = "切到 " + name
	return nil
}

// tuneStatus is the panel's small line: which group is heard and how many are
// saved, or that none is.
func (a *App) tuneStatus() string {
	if a.tuneIdx >= 0 && a.tuneIdx < len(a.tuneFiles) {
		return fmt.Sprintf("%d/%d 组", a.tuneIdx+1, len(a.tuneFiles))
	}
	if len(a.tuneFiles) > 0 {
		return fmt.Sprintf("%d 组", len(a.tuneFiles))
	}
	return "未保存"
}

// setFilterOn turns the look on or off without throwing the numbers away: off
// is the only level that takes the picture back to the core's own.
func (a *App) setFilterOn(on bool) {
	if on {
		if a.cfg.FilterLevel == "" || a.cfg.FilterLevel == "off" {
			a.cfg.FilterLevel = "normal"
		}
	} else {
		a.cfg.FilterLevel = "off"
	}
	a.saveConfig()
}
