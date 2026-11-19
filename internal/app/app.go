// Package app is the launcher: it reads the library, draws the window,
// and starts games.
//
// The window shows covers only. There is no video, no gif animation and
// no panel of game notes beside the grid: what a game is, the cover and
// the title already say.
package app

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/R19988088/GameRoom/internal/launch"
	"github.com/R19988088/GameRoom/internal/library"
)

// config is 数据/config.json: the little that differs between installs.
type config struct {
	// SimuiRoot is the old frontend's folder. It is read once, to import
	// its library, and may be empty afterwards.
	SimuiRoot string `json:"simuiRoot"`
	// CRTShader is the shader preset RetroArch runs with, relative to
	// 模拟器/RetroArch/shaders. Empty runs the game unfiltered.
	CRTShader string `json:"crtShader"`
}

// defaultShader is 脑浆油条's PhosphorLine: a CRT look that keeps arcade
// sprites readable at 4K, which is why it ships as the default. The preset is
// one file at the top of shaders/, and launch falls back to the pack it was
// taken from when that name is not there.
const defaultShader = "202603.slangp"

// App holds what the window reads each frame, and the library behind it.
type App struct {
	root string
	cfg  config

	db  *sql.DB
	lib *library.Library

	win   *mygo.Window
	theme ui.Theme

	group    string // the shelf shown: arcade, console, handheld, pc
	platform string // the chosen platform's key
	menu     string // "" shows every menu of the platform
	query    string
	chosen   int // the item chosen in the grid
	grid     ui.GridState

	// The two popovers of the top bar: the shelf picker on the left and the
	// platforms that did not make the bar's five.
	shelfOpen bool
	moreOpen  bool

	shown         []library.Game
	counts        map[string]int // games shown per platform
	menuCounts    map[string]int // games shown per menu of a.platform
	platformTotal int
	status        string

	covers  map[string]*ui.Bitmap // by path; nil once known missing
	icons   map[string]*ui.Bitmap
	pending map[string]bool

	// The cover-fetching run: how many covers the 街机 shelf is missing, and
	// whether a fetch is in flight, with its progress.
	missing  int
	fetching bool
	fetched  int
	fetchAll int
}

// Run starts the launcher, one instance at a time.
func Run() error {
	if !mygo.App.RequestSingleInstanceLock() {
		return nil
	}
	mygo.App.SetName("GameRoom")
	mygo.App.WhenReady(boot)
	return mygo.App.Run()
}

func boot() {
	a := &App{
		root:    findRoot(),
		status:  "就绪",
		covers:  map[string]*ui.Bitmap{},
		icons:   map[string]*ui.Bitmap{},
		pending: map[string]bool{},
	}
	a.loadConfig()
	a.theme = buildTheme()

	if err := a.open(); err != nil {
		fmt.Fprintln(os.Stderr, "open:", err)
		a.status = err.Error()
	}
	if a.lib != nil && len(a.lib.Platforms) > 0 {
		a.group = "arcade"
		key := ""
		for _, p := range a.lib.PlatformsOf(a.group) {
			key = p.Key
			break
		}
		if key == "" {
			a.group = a.lib.Platforms[0].Group
			key = a.lib.Platforms[0].Key
		}
		a.platform = key
		a.recount()
		a.refilter()
		a.countMissing()
	}

	a.win = mygo.NewWindow(mygo.WindowOptions{
		Title:     "GameRoom",
		Width:     1500,
		Height:    940,
		MinWidth:  1040,
		MinHeight: 660,
		Content:   ui.View(a.view),
	})
}

// findRoot is the folder the launcher runs from: the one holding 模拟器,
// rom and 数据. When started by `go run`, it is found above the source.
func findRoot() string {
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		candidates := []string{dir}
		if strings.EqualFold(filepath.Base(dir), "exe") {
			candidates = append(candidates, filepath.Dir(dir))
		}
		for _, c := range candidates {
			if hasLayout(c) {
				return c
			}
		}
	}
	if cwd, err := os.Getwd(); err == nil {
		dir := cwd
		for i := 0; i < 3; i++ {
			if hasLayout(dir) {
				return dir
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
		return cwd
	}
	return "."
}

func hasLayout(dir string) bool {
	for _, name := range []string{"rom", "模拟器", "数据"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
			return true
		}
	}
	return false
}

func (a *App) configPath() string { return filepath.Join(a.root, "数据", "config.json") }

func (a *App) loadConfig() {
	data, err := os.ReadFile(a.configPath())
	if err == nil {
		_ = json.Unmarshal(data, &a.cfg)
		if a.cfg.CRTShader == "" {
			a.cfg.CRTShader = defaultShader
			a.saveConfig()
		}
		return
	}
	a.cfg.CRTShader = defaultShader
	_ = os.MkdirAll(filepath.Dir(a.configPath()), 0o755)
	a.saveConfig()
}

func (a *App) saveConfig() {
	if data, err := json.MarshalIndent(a.cfg, "", "  "); err == nil {
		_ = os.WriteFile(a.configPath(), append(data, '\n'), 0o644)
	}
}

// open opens the library, importing the old frontend's the first time.
func (a *App) open() error {
	path := filepath.Join(a.root, "数据", "launcher.db")
	db, err := library.Open(path)
	if errors.Is(err, library.ErrOldSchema) {
		// The tables changed shape: rebuild from the old frontend's data,
		// when it is still there to read.
		if a.cfg.SimuiRoot != "" {
			for _, suffix := range []string{"", "-wal", "-shm"} {
				_ = os.Remove(path + suffix)
			}
			db, err = library.Open(path)
		}
	}
	if err != nil {
		return fmt.Errorf("打不开数据库：%w", err)
	}
	a.db = db

	if !library.Imported(db) {
		if a.cfg.SimuiRoot == "" {
			return fmt.Errorf("请先在 数据/config.json 写 simuiRoot，指向原 simUI 目录")
		}
		src := filepath.Join(filepath.FromSlash(a.cfg.SimuiRoot), "data.dll")
		if _, err := os.Stat(src); err != nil {
			return fmt.Errorf("找不到原库：%s", src)
		}
		if err := library.Import(db, src, filepath.FromSlash(a.cfg.SimuiRoot), a.root); err != nil {
			return fmt.Errorf("导入失败：%w", err)
		}
	}

	lib, err := library.Load(db)
	if err != nil {
		return fmt.Errorf("读库失败：%w", err)
	}
	a.lib = lib
	a.counts = lib.Counts()
	return nil
}

// countMissing counts the games the cover fetch would have to fill in, so the
// status line can offer the job instead of running it unasked.
func (a *App) countMissing() {
	a.missing = 0
	if a.lib == nil {
		return
	}
	a.missing = len(missingCovers(a.lib, a.root))
}

// fetchCovers downloads the missing covers behind the window, updating the
// status line as it goes, then reloads the library so the grid shows them.
func (a *App) fetchCovers() {
	if a.fetching || a.lib == nil {
		return
	}
	targets := missingCovers(a.lib, a.root)
	if len(targets) == 0 {
		a.status = "封面都是齐的"
		return
	}
	a.fetching, a.fetched, a.fetchAll = true, 0, len(targets)
	a.status = fmt.Sprintf("补全封面 0/%d", a.fetchAll)

	go func() {
		ok, failed := downloadCovers(a.db, a.root, targets, func(done, total, ok int) {
			a.win.Update(func() {
				a.fetched = done
				a.status = fmt.Sprintf("补全封面 %d/%d · 已新增 %d", done, total, ok)
			})
		})
		a.win.Update(func() {
			a.fetching = false
			a.status = fmt.Sprintf("封面补全完成：新增 %d，失败 %d", ok, failed)
			a.reload()
		})
	}()
}

// reload rereads the library after a write, keeping the shelf and menu shown.
func (a *App) reload() {
	lib, err := library.Load(a.db)
	if err != nil {
		a.status = "读库失败：" + err.Error()
		return
	}
	a.lib = lib
	a.counts = lib.Counts()
	a.covers = map[string]*ui.Bitmap{}
	a.pending = map[string]bool{}
	a.countMissing()
	a.recount()
	a.refilter()
}

// refilter rebuilds the grid's games: the platform's, the menu's, the
// search's.
func (a *App) refilter() {
	a.shown = a.shown[:0]
	if a.lib != nil {
		q := strings.ToLower(strings.TrimSpace(a.query))
		for _, g := range a.lib.Games {
			if g.PlatformKey != a.platform {
				continue
			}
			if a.menu != "" && g.Menu != a.menu {
				continue
			}
			if q != "" && !strings.Contains(strings.ToLower(g.Title), q) {
				continue
			}
			a.shown = append(a.shown, g)
		}
	}
	a.chosen = 0
	a.grid.ScrollIntoView(0)
}

func (a *App) choosePlatform(key string) {
	if a.platform == key {
		return
	}
	a.platform, a.menu, a.query = key, "", ""
	a.moreOpen = false
	a.recount()
	a.refilter()
}

// chooseGroup switches the shelf; the first platform on it becomes the
// one shown.
func (a *App) chooseGroup(group string) {
	if a.group == group || a.lib == nil {
		return
	}
	a.group, a.menu, a.query = group, "", ""
	a.shelfOpen, a.moreOpen = false, false
	for _, p := range a.lib.PlatformsOf(group) {
		a.platform = p.Key
		a.recount()
		a.refilter()
		return
	}
	// An empty shelf shows nothing rather than the last shelf's games.
	a.platform = ""
	a.menuCounts = map[string]int{}
	a.platformTotal = 0
	a.refilter()
}

// recount counts the games of the chosen platform, and of each of its
// menus, as the sidebar shows them.
func (a *App) recount() {
	a.menuCounts = map[string]int{}
	a.platformTotal = 0
	if a.lib == nil {
		return
	}
	for _, g := range a.lib.Games {
		if g.PlatformKey != a.platform {
			continue
		}
		a.platformTotal++
		if g.Menu != "" {
			a.menuCounts[g.Menu]++
		}
	}
}

func (a *App) chooseMenu(name string) {
	if a.menu == name {
		return
	}
	a.menu = name
	a.refilter()
}

func (a *App) platformName() string {
	if a.lib == nil {
		return "GameRoom"
	}
	if p := a.lib.Platform(a.platform); p != nil {
		return p.Name
	}
	return "GameRoom"
}

// launchGame starts the chosen game, when its emulator is RetroArch.
func (a *App) launchGame(g library.Game) {
	if a.lib == nil {
		return
	}
	sim := a.lib.SimFor(g)
	if sim == nil {
		a.status = g.Title + "：没有配置模拟器"
		return
	}
	if !launch.IsRetroArch(sim.Exe) {
		a.status = fmt.Sprintf("%s：该游戏用 %s（本版只启动 RetroArch）", g.Title, sim.Name)
		return
	}
	// A game may name a core of its own; the sim's is the fallback.
	if g.Core != "" {
		s := *sim
		s.Core = g.Core
		sim = &s
	}

	rom := filepath.Join(a.root, "rom", filepath.FromSlash(g.RomPath))
	t, err := launch.Resolve(a.root, *sim, rom)
	if err != nil {
		a.status = err.Error()
		return
	}
	// The CRT preset goes in as RetroArch's global shader; a missing one
	// must not stop the game.
	_ = launch.InstallShader(t, a.cfg.CRTShader)
	cmd, err := launch.Start(t)
	if err != nil {
		a.status = "启动失败：" + err.Error()
		return
	}

	name := g.Title
	a.status = "已启动：" + name
	if !launch.IsRetroArch(sim.Exe) {
		a.status = "已启动：" + name + "（交给 Magpie 套 CRT）"
		go launch.TakeOver(cmd.Process.Pid, filepath.Base(t.Exe), launch.MagpieFilter{Exe: a.magpieExe()})
	}
	go func() {
		err := cmd.Wait()
		a.win.Update(func() {
			if err != nil {
				a.status = name + " 退出：" + err.Error()
			} else {
				a.status = name + " 已退出"
			}
		})
	}()
}

// magpieExe is where the CRT filter for external programs lives; empty means
// the folder has none and only window management happens.
func (a *App) magpieExe() string {
	exe := filepath.Join(a.root, "模拟器", "Magpie", "Magpie.exe")
	if _, err := os.Stat(exe); err != nil {
		return ""
	}
	return exe
}

func (a *App) launchChosen() {
	if a.chosen < 0 || a.chosen >= len(a.shown) {
		return
	}
	a.launchGame(a.shown[a.chosen])
}

// coverAbs turns a cover's path in the library into a file path.
func (a *App) coverAbs(cover string) string {
	if cover == "" {
		return ""
	}
	return filepath.Join(a.root, "数据", filepath.FromSlash(cover))
}

// cover loads a cover once, in the background, at the size the grid
// shows it. A missing file is remembered as missing, not retried.
func (a *App) cover(path string) *ui.Bitmap {
	if path == "" {
		return nil
	}
	if b, ok := a.covers[path]; ok {
		return b
	}
	if a.pending[path] {
		return nil
	}
	a.pending[path] = true
	go func() {
		var b *ui.Bitmap
		if data, err := os.ReadFile(path); err == nil {
			b, _ = ui.DecodeBitmapScaled(data, 420)
		}
		a.win.Update(func() { a.covers[path] = b })
	}()
	return nil
}

// icon loads a platform's icon, small.
func (a *App) icon(path string) *ui.Bitmap {
	if path == "" {
		return nil
	}
	if b, ok := a.icons[path]; ok {
		return b
	}
	if a.pending[path] {
		return nil
	}
	a.pending[path] = true
	go func() {
		var b *ui.Bitmap
		if data, err := os.ReadFile(path); err == nil {
			b, _ = ui.DecodeBitmapScaled(data, 64)
		}
		a.win.Update(func() { a.icons[path] = b })
	}()
	return nil
}

// buildTheme is the dark, warm palette the launcher draws with: near
// black surfaces, hairlines, and one orange accent for what is chosen.
func buildTheme() ui.Theme {
	t := *ui.DarkTheme()
	t.Background = ui.Hex("#0f0f12")
	t.Surface = ui.Hex("#191a20")
	t.SurfaceHover = ui.Hex("#22232b")
	t.SurfacePressed = ui.Hex("#2b2c36")
	t.Border = ui.Hex("#26272f")
	t.Text = ui.Hex("#ececf1")
	t.TextMuted = ui.Hex("#8b8b99")
	t.Accent = ui.Hex("#e8763a")
	t.AccentHover = ui.Hex("#f28a52")
	t.AccentPressed = ui.Hex("#cf6430")
	t.AccentText = ui.Hex("#141416")
	t.Selection = ui.Hex("#3a2a22")
	t.Focus = ui.Hex("#e8763a")
	t.Radius = 10
	t.Spacing = 8
	return t
}
