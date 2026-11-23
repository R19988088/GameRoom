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
// and the numbers the player had moved. A group is a whole look, so selecting
// it brings back both the preset and the sliders.
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

// tuneMax is the highest number the saved groups reach.
func (a *App) tuneMax() int {
	max := 0
	for _, name := range a.tuneFiles {
		if n := tuneNumber(name); n > max {
			max = n
		}
	}
	return max
}

// writeTune puts the numbers being heard into the named group's file and makes
// it the one the config remembers, so the card stays lit after a restart.
func (a *App) writeTune(name string) error {
	if a.root == "" {
		return errors.New("没有数据目录")
	}
	if err := os.MkdirAll(a.tuneDir(), 0o755); err != nil {
		return err
	}
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
	return nil
}

// newTune starts a group after the last one out of the numbers being heard and
// leaves its card lit: 保存 does it when no card is lit, 另存 when one is. A card
// is only added once its file has landed, so a failed write leaves no empty card.
func (a *App) newTune() error {
	if a.root == "" {
		return errors.New("没有数据目录")
	}
	idx := len(a.tuneFiles)
	name := fmt.Sprintf("tune-%d.json", a.tuneMax()+1)
	if err := a.writeTune(name); err != nil {
		return err
	}
	a.tuneFiles = append(a.tuneFiles, name)
	a.tuneIdx = idx
	return nil
}

// updateTune writes the numbers being heard back into the card that is lit.
func (a *App) updateTune() error {
	if a.tuneIdx < 0 || a.tuneIdx >= len(a.tuneFiles) {
		return errors.New("还没有选中卡片")
	}
	return a.writeTune(a.tuneFiles[a.tuneIdx])
}

// selectTune takes the group the card at i stands for: its preset and its
// numbers, so the panel shows what is heard.
func (a *App) selectTune(i int) error {
	if i < 0 || i >= len(a.tuneFiles) {
		return errors.New("没有这张卡片")
	}
	name := a.tuneFiles[i]
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
	a.tuneIdx = i
	a.dragKey = ""
	a.saveConfig()
	return nil
}

// deleteTune removes the lit card and takes the one that slides into its place,
// so the panel keeps showing a saved group while any is left. With none left the
// numbers stay on the sliders as they are, and 保存 comes back.
func (a *App) deleteTune() error {
	if a.tuneIdx < 0 || a.tuneIdx >= len(a.tuneFiles) {
		return errors.New("还没有选中卡片")
	}
	idx := a.tuneIdx
	name := a.tuneFiles[idx]
	if err := os.Remove(filepath.Join(a.tuneDir(), name)); err != nil {
		return err
	}
	a.tuneFiles = append(a.tuneFiles[:idx], a.tuneFiles[idx+1:]...)
	a.tuneIdx = -1
	a.cfg.FilterTune = ""
	if len(a.tuneFiles) == 0 {
		a.saveConfig()
		return nil
	}
	if idx >= len(a.tuneFiles) {
		idx = len(a.tuneFiles) - 1
	}
	return a.selectTune(idx)
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
