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
	"slices"
	"sort"
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

	group    string // the shelf shown: arcade, mahjong, console, handheld, pc
	axis     string // how the shelf files its games: board, maker, machine, year
	platform string // the chosen platform's key, "" when the axis has no machine
	menu     string // "" shows every game of the shelf
	query    string
	chosen   int // the card chosen in the body: an index into shown
	scroll   ui.ScrollState

	// The two popovers of the top bar: the shelf picker on the left and the
	// platforms that did not make the bar's five.
	shelfOpen bool
	moreOpen  bool

	// The card's own menu, opened with the right button: one open flag per
	// game (so each card owns its menu), which game it belongs to, and what it
	// is showing — a field's values to choose, or a text field to type one in.
	opens     map[int]*bool
	menuGame  int
	hoverGame int
	cardField string
	cardEdit  bool
	cardText  string

	shown []library.Game
	// The body, filed by the standard the bar shows: sections with their
	// header, and the lines of cards that make them up. lines are laid out
	// again when the shelf, the standard or the window width changes.
	sections      []section
	lines         []line
	cols          int
	contentH      float32
	lastH         float32
	stale         bool
	counts        map[string]int // games shown per platform
	menuCounts    map[string]int // games shown per value of the axis
	chips         []library.Chip // the axis' values, in bar order
	platformTotal int            // games of the chosen platform
	shelfTotal    int            // games of the whole shelf
	status        string

	covers  map[string]*ui.Bitmap // by path; nil once known missing
	icons   map[string]*ui.Bitmap
	pending map[string]bool

	// The decode queue: pictures wait here and are decoded one at a time.
	jobs    []imageJob
	loading bool

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
		a.axis = a.defaultAxis()
		a.lib.SplitGroup(a.group, a.axis)
		a.platform = a.biggestPlatform(a.group)
		if a.platform == "" {
			a.group = a.lib.Platforms[0].Group
			a.axis = a.defaultAxis()
			a.lib.SplitGroup(a.group, a.axis)
			a.platform = a.biggestPlatform(a.group)
		}
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
	// Load put every shelf back on its default standard; restore the one the
	// user was looking at.
	if a.axis != "" {
		a.lib.SplitGroup(a.group, a.axis)
	}
	a.countMissing()
	a.recount()
	a.refilter()
}

// refilter rebuilds the grid's games: those of the shelf, of the axis and of
// the search.
func (a *App) refilter() {
	a.shown = a.shown[:0]
	if a.lib != nil {
		q := strings.ToLower(strings.TrimSpace(a.query))
		machine := a.axis == library.AxisMachine || a.axis == ""
		for _, g := range a.lib.Games {
			p := a.lib.Platform(g.PlatformKey)
			if p == nil || p.Group != a.group {
				continue
			}
			if machine {
				if g.PlatformKey != a.platform {
					continue
				}
			} else if a.menu != "" && g.Menu != a.menu {
				continue
			}
			if q != "" && !strings.Contains(strings.ToLower(g.Title), q) {
				continue
			}
			a.shown = append(a.shown, g)
		}
	}
	a.file()
	a.chosen = 0
	a.scroll.Y = 0
}

// section is one group of the body: the value of the chosen standard that
// files it, and the games it holds, a run of shown.
type section struct {
	name  string // the chip's name, "" when the shelf has one unnamed group
	from  int    // first game in shown
	count int
}

// line is one row the body draws: a section's header when head >= 0, else a
// row of at most cols cards, named by their places in shown.
type line struct {
	top   float32
	head  int   // index into sections, -1 on a row of cards
	games []int // indices into shown, nil on a header
}

// file orders the games on screen by the standard the bar shows, and cuts
// them into the sections the body labels. On a shelf whose standard is its
// platforms (家用机 on 机种) there is nothing to group and nothing to label.
func (a *App) file() {
	a.sections, a.stale = a.sections[:0], true
	if len(a.shown) == 0 {
		return
	}
	if len(a.chips) == 0 {
		// One unnamed group (家用机 on 机种: the platform is the machine). It
		// has nothing to label, but it is still filed by name.
		sort.SliceStable(a.shown, func(i, j int) bool {
			return titleLess(a.shown[i].Title, a.shown[j].Title)
		})
		a.sections = append(a.sections, section{from: 0, count: len(a.shown)})
		return
	}
	// The bar's order is the body's: busiest value first, unknown last; inside
	// a group the cards are filed by their names, so a group of 51 is not a
	// wall of noise but a list one can scan.
	rank := make(map[string]int, len(a.chips))
	for i, c := range a.chips {
		rank[c.Name] = i
	}
	sort.SliceStable(a.shown, func(i, j int) bool {
		ri, rok := rank[a.shown[i].Menu]
		rj, jok := rank[a.shown[j].Menu]
		if !rok {
			ri = len(a.chips)
		}
		if !jok {
			rj = len(a.chips)
		}
		if ri != rj {
			return ri < rj
		}
		return titleLess(a.shown[i].Title, a.shown[j].Title)
	})
	for i := 0; i < len(a.shown); {
		j := i + 1
		for j < len(a.shown) && a.shown[j].Menu == a.shown[i].Menu {
			j++
		}
		a.sections = append(a.sections, section{name: a.shown[i].Menu, from: i, count: j - i})
		i = j
	}
}

// menuOpen is the open flag of one card's menu, made the first time the card
// is built: a card owns its flag, so the popup anchors to the card that was
// right clicked rather than to whichever card was built last.
func (a *App) menuOpen(id int) *bool {
	if a.opens == nil {
		a.opens = map[int]*bool{}
	}
	p, ok := a.opens[id]
	if !ok {
		p = new(bool)
		a.opens[id] = p
	}
	return p
}

// menuFor makes this card the one the open menu belongs to, and closes the
// other cards' flags — clearing what the menu was showing.
func (a *App) menuFor(id int) {
	if a.menuGame == id {
		return
	}
	for k, p := range a.opens {
		if k != id {
			*p = false
		}
	}
	a.menuGame, a.cardField, a.cardEdit, a.cardText = id, "", false, ""
}

// shutMenu closes the card's menu and forgets what it showed.
func (a *App) shutMenu() {
	for _, p := range a.opens {
		*p = false
	}
	a.menuGame, a.cardField, a.cardEdit, a.cardText = 0, "", false, ""
}

// values is what a field could be set to: the values the shelf already
// carries, so a wrong 厂商 is corrected by picking a right one, and a value
// nobody has yet is typed in instead.
func (a *App) values(field string, g library.Game) []string {
	if a.lib == nil {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	add := func(v string) {
		if v != "" && !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	p := a.lib.Platform(g.PlatformKey)
	switch field {
	case "machine":
		if p != nil && p.Group == "handheld" {
			for _, q := range a.lib.Games {
				if q.PlatformKey == g.PlatformKey {
					add(a.lib.MachineNameOf(q))
				}
			}
		} else {
			for _, q := range a.lib.PlatformsOf(a.group) {
				add(q.Name)
			}
		}
	case "core":
		for i := range a.lib.Sims {
			if a.lib.Sims[i].PlatformKey == g.PlatformKey {
				add(a.lib.Sims[i].Core)
			}
		}
		for _, q := range a.lib.Games {
			if q.PlatformKey == g.PlatformKey {
				add(q.Core)
			}
		}
	case "board", "publisher", "genre":
		for _, q := range a.lib.Games {
			qp := a.lib.Platform(q.PlatformKey)
			if qp == nil || qp.Group != a.group {
				continue
			}
			switch field {
			case "board":
				add(library.BoardNameOf(q))
			case "publisher":
				add(a.lib.MakerOf(q))
			case "genre":
				add(q.Genre)
			}
		}
	}
	slices.Sort(out)
	return out
}

// assign writes corrected metadata for the game the card's menu was opened on:
// into the library in memory, into the row of the database, and into the body
// at once, so the card moves to its new 厂商 or 基板 without a restart.
func (a *App) assign(field, value string) {
	v := strings.TrimSpace(value)
	g := a.lib.Game(a.menuGame)
	if g == nil || v == "" {
		return
	}
	var err error
	switch field {
	case "board":
		g.Board = v
		err = a.write(g.ID, library.FieldBoard, v)
	case "publisher":
		g.Publisher = v
		err = a.write(g.ID, library.FieldPublisher, v)
	case "genre":
		g.Genre = v
		err = a.write(g.ID, library.FieldGenre, v)
	case "core":
		g.Core = v
		err = a.write(g.ID, library.FieldCore, v)
	case "machine":
		if p := a.lib.Platform(g.PlatformKey); p != nil && p.Group == "handheld" {
			// 掌机 的机种由核心决定：换成哪台机器，就是换成哪个核心。
			core := library.CoreOfMachine(v)
			if core == "" {
				core = v
			}
			g.Core = core
			err = a.write(g.ID, library.FieldCore, core)
		} else {
			// 家用机 的机种就是平台：改机种等于把这张卡换到那台机器上。
			for _, q := range a.lib.PlatformsOf(a.group) {
				if q.Name == v || q.Key == v {
					g.PlatformKey = q.Key
					if a.db != nil {
						err = library.SetPlatform(a.db, g.ID, q.Key)
					}
					break
				}
			}
		}
	}
	name := strings.ToUpper(g.Title)
	if err != nil {
		a.status = fmt.Sprintf("%s 没改成：%v", name, err)
	} else {
		a.status = fmt.Sprintf("%s 已改为 %s", name, v)
	}
	a.counts = a.lib.Counts()
	a.lib.SplitGroup(a.group, a.axis)
	a.recount()
	// The card may move to another group, but the window stays where it was:
	// correcting one game should not throw away the place in the list.
	at := a.scroll.Y
	a.refilter()
	a.scroll.Y = at
	a.shutMenu()
}

// write is one column of one game into the database, when the launcher has
// one: a test builds an App without it.
func (a *App) write(id int, field, value string) error {
	if a.db == nil {
		return nil
	}
	return library.SetField(a.db, id, field, value)
}

// titleLess files a group by its games' names: a run of digits compares as a
// number, so 街霸2 comes before 街霸10, and everything else compares as text,
// which keeps the Chinese titles in a stable order.
func titleLess(a, b string) bool {
	ka, kb := naturalKey(a), naturalKey(b)
	for i := 0; i < len(ka) && i < len(kb); i++ {
		if ka[i] != kb[i] {
			return ka[i] < kb[i]
		}
	}
	return len(ka) < len(kb)
}

// naturalKey lowercases a title and turns its digit runs into numbers, so the
// comparison above is a natural one. Numeric runs are marked with a lead byte
// below every letter, which keeps numbers ahead of words at the same spot.
func naturalKey(s string) []string {
	s = strings.ToLower(strings.TrimSpace(s))
	var parts []string
	for i := 0; i < len(s); {
		j := i
		digit := s[i] >= '0' && s[i] <= '9'
		for j < len(s) && (s[j] >= '0' && s[j] <= '9') == digit {
			j++
		}
		part := s[i:j]
		if digit {
			n := strings.TrimLeft(part, "0")
			if n == "" {
				n = "0"
			}
			// Pad to one width so 9 sorts before 10 as text compares.
			for len(n) < 12 {
				n = "0" + n
			}
			parts = append(parts, "\x01"+n)
		} else {
			parts = append(parts, part)
		}
		i = j
	}
	return parts
}

// choosePlatform shows one platform of the shelf.
func (a *App) choosePlatform(key string) {
	if a.platform == key {
		return
	}
	a.platform, a.menu, a.query = key, "", ""
	a.moreOpen = false
	a.recount()
	a.refilter()
}

// axes are the standards the shelf offers.
func (a *App) axes() []string { return library.AxesOf(a.group) }

// defaultAxis is the standard a shelf opens on: the first it offers.
func (a *App) defaultAxis() string {
	if ax := a.axes(); len(ax) > 0 {
		return ax[0]
	}
	return library.AxisMachine
}

// chooseAxis switches how the shelf files its games, the switch on the bar.
func (a *App) chooseAxis(axis string) {
	if a.axis == axis {
		return
	}
	a.axis, a.menu, a.moreOpen = axis, "", false
	if a.lib != nil {
		a.lib.SplitGroup(a.group, axis)
		if axis == library.AxisMachine {
			// The 机种 axis needs a platform on screen, and the shelf's
			// busiest one is what a person expects to land on.
			a.platform = a.biggestPlatform(a.group)
		}
	}
	a.recount()
	a.refilter()
}

// biggestPlatform is the platform of a shelf with the most games, so entering
// a shelf (or its 机种 axis) does not open on a one-game machine.
func (a *App) biggestPlatform(group string) string {
	if a.lib == nil {
		return ""
	}
	best, bestN := "", -1
	counts := a.lib.Counts()
	for _, p := range a.lib.PlatformsOf(group) {
		if n := counts[p.Key]; n > bestN {
			best, bestN = p.Key, n
		}
	}
	return best
}

// nextAxis is the switch's click: the other standard of the shelf.
func (a *App) nextAxis() {
	ax := a.axes()
	if len(ax) < 2 {
		return
	}
	for i, x := range ax {
		if x == a.axis {
			a.chooseAxis(ax[(i+1)%len(ax)])
			return
		}
	}
	a.chooseAxis(ax[0])
}

// chooseGroup switches the shelf; the first platform on it becomes the
// one shown.
func (a *App) chooseGroup(group string) {
	if a.group == group || a.lib == nil {
		return
	}
	a.group, a.menu, a.query = group, "", ""
	a.axis = a.defaultAxis()
	a.shelfOpen, a.moreOpen = false, false
	a.lib.SplitGroup(group, a.axis)
	a.platform = a.biggestPlatform(group)
	a.recount()
	a.refilter()
}

// recount counts what the bar shows: the games of each platform, of the whole
// shelf, and of each value of the axis.
func (a *App) recount() {
	a.menuCounts = map[string]int{}
	a.platformTotal, a.shelfTotal = 0, 0
	if a.lib == nil {
		return
	}
	for _, g := range a.lib.Games {
		p := a.lib.Platform(g.PlatformKey)
		if p == nil || p.Group != a.group {
			continue
		}
		a.shelfTotal++
		if g.PlatformKey == a.platform {
			a.platformTotal++
		}
		if g.Menu != "" {
			a.menuCounts[g.Menu]++
		}
	}
	a.chips = a.lib.Chips(a.group, a.axis)
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

// scopeName is what the status bar calls the games on screen: the platform
// when the shelf is filed by machine, otherwise the shelf and the value
// chosen on it.
func (a *App) scopeName() string {
	if a.axis == library.AxisMachine || a.axis == "" {
		return a.platformName()
	}
	name := ""
	for _, g := range library.Groups {
		if g.Key == a.group {
			name = g.Name
		}
	}
	value := a.menu
	if value == "" {
		value = "全部"
	}
	return name + " · " + value
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

// cover loads a cover once, at the size the grid shows it. A missing file is
// remembered as missing, not retried.
func (a *App) cover(path string) *ui.Bitmap {
	return a.image(path, 420, false)
}

// icon loads a platform's icon, small.
func (a *App) icon(path string) *ui.Bitmap {
	return a.image(path, 64, true)
}

// imageJob is one picture waiting its turn in the decode queue.
type imageJob struct {
	path string
	max  int
	icon bool
}

// image asks for a picture: it is returned at once when it is already in
// memory, otherwise it is queued and decoded later, one at a time. A path
// already queued or already tried is not asked for twice.
func (a *App) image(path string, max int, icon bool) *ui.Bitmap {
	if path == "" {
		return nil
	}
	var have map[string]*ui.Bitmap
	if icon {
		have = a.icons
	} else {
		have = a.covers
	}
	if b, ok := have[path]; ok {
		return b
	}
	if a.pending[path] {
		return nil
	}
	a.pending[path] = true
	a.jobs = append(a.jobs, imageJob{path: path, max: max, icon: icon})
	a.pumpImages()
	return nil
}

// pumpImages decodes the queue strictly one picture at a time: scrolling fast
// must not open a decoder per visible card. Each finished picture asks for the
// next, so the order the grid asked in is the order they arrive.
func (a *App) pumpImages() {
	if a.loading || len(a.jobs) == 0 {
		return
	}
	job := a.jobs[0]
	a.jobs = a.jobs[1:]
	a.loading = true
	go func() {
		var b *ui.Bitmap
		if data, err := os.ReadFile(job.path); err == nil {
			b, _ = ui.DecodeBitmapScaled(data, job.max)
		}
		done := func() {
			if job.icon {
				a.icons[job.path] = b
			} else {
				a.covers[job.path] = b
			}
			a.loading = false
			a.pumpImages()
		}
		if a.win == nil {
			done()
			return
		}
		a.win.Update(done)
	}()
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
