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
	"image"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/audio"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/shader"

	"github.com/R19988088/GameRoom/internal/core"

	"github.com/R19988088/GameRoom/internal/ips"
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
	// FilterLevel bends that preset: "" or "normal" is the preset as its
	// author tuned it, "soft" and "strong" are the lighter and heavier looks
	// the player can try, and "off" runs the game unfiltered.
	FilterLevel string `json:"filterLevel"`
	// CRTTune are the preset's parameters as the player moved them, by the
	// name the preset sets them under; missing ones stay as the preset has
	// them.
	CRTTune map[string]float32 `json:"crtTune,omitempty"`
	// FilterTune is the saved group under 数据/filters the player last heard or
	// saved, by file name. Empty means none has been picked.
	FilterTune string `json:"filterTune,omitempty"`
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
	// The bar lists five categories and folds the rest under 其他, with 收藏
	// before them when the shelf has starred cards. special says which of the
	// two is standing in for a chip's name: "" for a chip (or 全部).
	special string
	query   string
	chosen  int // the card chosen in the body: an index into shown
	scroll  ui.ScrollState

	// A second click on the same card within this long starts the game. The
	// window's own double-click counting is not relied on: a card is built
	// again every frame, and a counter keyed by the element can miss the pair.
	lastClick   int
	lastClickAt time.Time

	// Work that changes the body's games, queued for the top of the next frame:
	// a card's menu is drawn from the lines of this frame, so changing the
	// filter or the metadata right there leaves those lines pointing past the
	// end of a shorter list, and the window dies.
	queue []func()

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

	// The version picker under a card: which family every game belongs to, the
	// dumps of each family, the one chosen to play, and the popup the card
	// opened — one open flag per game, like the card's own menu. vmenu is the
	// right-click menu of one of those rows (取消合并), with its own flags so
	// that the card's menu and a row's never light each other up.
	verFam     map[int]string
	verMembers map[string][]library.Game
	verDefault map[string]int
	verOpen    map[int]*bool
	verGame    int
	verSel     int
	vmenu      map[int]*bool
	vmenuGame  int

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
	favTotal      int            // starred games of the whole shelf
	otherTotal    int            // games of the shelf that 其他 holds
	status        string

	covers  map[string]*ui.Bitmap // by path; nil once known missing
	icons   map[string]*ui.Bitmap
	pending map[string]bool
	// imgMu guards the three maps and the queue above: a decoder finishes on
	// its own goroutine and, with no window to post to (the headless tester),
	// writes the finished picture straight from there while the frame is being
	// built. Go's maps die on that — "fatal error: concurrent map read and map
	// write" — and a fatal error takes the whole launcher down with it.
	imgMu sync.Mutex

	// The decode queue: pictures wait here and are decoded one at a time.
	jobs    []imageJob
	loading bool

	// The cover-fetching run: how many covers the 街机 shelf is missing, and
	// whether a fetch is in flight, with its progress.
	missing  int
	fetching bool
	fetched  int
	fetchAll int

	// The play area: while a game runs this window shows the game's own picture,
	// drawn by the core this process loaded, with the two controls over it. The
	// bitmap is the picture an element of the page; filtersOpen shows the filter
	// picker, which needs the play area for itself.
	core         *core.Session
	gameBmp      *ui.Bitmap
	gameImg      *image.RGBA
	gameFrames   int
	crt          crtParams
	fxShader     *shader.Shader
	dragKey      string
	dragVal      float64
	tunePtrs     map[string]*float64
	gameSound    *audio.Player
	frameW       int
	frameH       int
	gameAspect   float32
	mouseX       float64
	mouseY       float64
	mouseSeen    bool
	player       *launch.Player
	playerTarget launch.Target
	playerGame   library.Game
	// A game running in RetroArch's own window: the MAME cores cannot be
	// stepped from this process (they take the rom's parent folder as the
	// system name), so those games are handed to the frontend that can.
	external    *exec.Cmd
	filtersOpen bool

	// The filter panel's saved groups: tune-<n>.json under 数据/filters, the one
	// being heard (tuneIdx, -1 for none), and a line for the panel to say what a
	// save or a switch did.
	tuneFiles []string
	tuneIdx   int
	tuneNote  string

	// The one group of controls scrolls: the state keeps its place while a
	// parameter is dragged, so the list does not jump back to the top.
	filterScroll ui.ScrollState

	// The pad panel: what a machine's buttons read from. pads is the profile
	// saved per machine in 数据/controls.json, and padLive the machine the
	// running game reads (set at launch) — the panel follows the game, so there
	// is nothing to pick by hand. cap is the button waiting for a key, capP the
	// player whose column it belongs to, with what was already held when the
	// wait began, so a key pressed before it does not count.
	pads     map[string]map[string]padBind
	padsOpen bool
	padLive  string
	// padIDs is what the running core calls each button of the panel in use
	// (padIDsFor): the core announces its own names for the machine's buttons,
	// and those decide which retropad id a spot presses. nil means the panel's
	// drawn ids are the ones the core reads (MAME, or a core that says nothing).
	padIDs map[string][]int
	// padNow is the panel drawn while a game runs, when the core reads more
	// buttons than the board's model was written with (padPanelFor). nil means
	// the model padLive names is the one on screen.
	padNow *padLayout
	// padLogHead is the sheet 数据/按键自检.txt opens with — the core's own button
	// names, the panel, and what each key presses — and padLog the last press written
	// to it, so a line only lands when the pressed keys change. padGame/padCore are
	// what the sheet is about, kept so it can be written again when the core speaks
	// late (padLate); padIDsDone says whether that second read already happened.
	padLogHead string
	padLog     string
	padGame    library.Game
	padCore    string
	padIDsDone bool
	// Game-specific pad profiles: bindings filed under a game's own key, and
	// whether that game still reads the platform-wide profile beside it. A game
	// with no own entries follows its platform, exactly as before.
	gamePads    map[string]map[string]padBind
	gameGeneric map[string]bool
	padNote     string
	cap         string
	capP        int
	capVK       map[int]bool
	capPad      map[int]map[int]bool
	escDown     bool
	// The stick's hold key, one state per player: on is the direction it is
	// keeping on ("" for none), down whether the key itself is down, so only the
	// press counts and not the repeat.
	hold [2]padHold

	// The save panel and its two halves: slots is the shelf read off 数据/存档
	// (the picture and the line beside it), shots the decoded pictures, kept so a
	// tile does not decode a PNG every frame, and slotSel the tile a click left
	// selected. rew is the row of states the timeline steps back through, scrub
	// the drag of that timeline, trackW how wide it was drawn.
	savesOpen bool
	slots     []*saveSlot
	shots     map[string]*ui.Bitmap
	slotSel   int
	slotNote  string
	escSaves  bool
	rew       rewind
	scrub     scrubDrag
	trackW    float32
}

// padHold is one player's stick hold: the direction kept on, and whether the
// hold key was already down on the last frame.
type padHold struct {
	on   string
	down bool
}

// Run starts the launcher, one instance at a time.
func Run() error {
	if !mygo.App.RequestSingleInstanceLock() {
		return nil
	}
	mygo.App.SetName("GameRoom")
	// 再点一次图标时，第二个实例会把命令行交给已经开着的这个：把它提到前面，
	// 否则什么都不发生，看着就是「点了没反应、没有窗口」。
	mygo.App.OnSecondInstance(func([]string, string) { raiseWindow() })
	mygo.App.WhenReady(boot)
	return mygo.App.Run()
}

// running is the launcher's own App, for the second instance to raise its
// window: boot sets it once the window exists.
var running *App

// raiseWindow brings the launcher's window back: restored if it was minimized,
// shown if it was hidden, and focused.
func raiseWindow() {
	if running == nil || running.win == nil {
		return
	}
	running.win.Restore()
	running.win.Show()
	running.win.Focus()
}

// autoPlay is a game to start as soon as the window is up, named by -play.
// It exists so that "does the game really run inside the window" can be
// checked without a person clicking a card.
var autoPlay string

// AutoPlay names the game -play should start.
func AutoPlay(match string) { autoPlay = match }

// autoplay opens the first game matching autoPlay, after the window has drawn.
func (a *App) autoplay() {
	if autoPlay == "" || a.lib == nil {
		return
	}
	m := strings.ToLower(autoPlay)
	for _, g := range a.lib.Games {
		if !strings.Contains(strings.ToLower(g.Title), m) &&
			!strings.Contains(strings.ToLower(g.RomPath), m) {
			continue
		}
		game := g
		time.AfterFunc(2*time.Second, func() {
			a.win.Update(func() { a.launchGame(game) })
		})
		return
	}
	a.status = "找不到要自动运行的游戏：" + autoPlay
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
	a.loadPads()
	// The look is the CRT preset's own, read from the file the settings name,
	// so the picture is the one the library has always shown.
	a.crt = loadCRT(a.root, a.cfg.CRTShader)
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
		a.buildVersions()
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
	running = a
	a.autoplay()
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
	defer a.scanTunes()
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
	if a.db == nil {
		a.status = "读库失败：没有可读的库"
		return
	}
	lib, err := library.Load(a.db)
	if err != nil {
		a.status = "读库失败：" + err.Error()
		return
	}
	a.lib = lib
	a.counts = lib.Counts()
	a.imgMu.Lock()
	a.covers = map[string]*ui.Bitmap{}
	a.pending = map[string]bool{}
	a.imgMu.Unlock()
	// Load put every shelf back on its default standard; restore the one the
	// user was looking at.
	if a.axis != "" {
		a.lib.SplitGroup(a.group, a.axis)
	}
	a.countMissing()
	a.recount()
	a.buildVersions()
	a.refilter()
}

// later queues work that changes the body's games, to run at the top of the
// next frame, before the lines are cut again.
func (a *App) later(fn func()) {
	a.queue = append(a.queue, fn)
}

// runQueue runs what the last frame queued. It is called before anything is
// drawn, so every line of the new frame is cut from the games as they are now.
func (a *App) runQueue() {
	if len(a.queue) == 0 {
		return
	}
	q := a.queue
	a.queue = a.queue[:0]
	for _, fn := range q {
		fn()
	}
}

// The two pages the bar stands for besides a chip: the starred cards, and the
// categories the bar's five chips left out.
const (
	specialFav   = "fav"
	specialOther = "other"
)

// refilter rebuilds the grid's games: those of the shelf, of the axis and of
// the search.
func (a *App) refilter() {
	a.shown = a.shown[:0]
	if a.lib != nil {
		q := strings.ToLower(strings.TrimSpace(a.query))
		machine := a.axis == library.AxisMachine || a.axis == ""
		// 其他 is the shelf minus the bar's five: on a shelf whose machine is
		// its platform that is the machines left out, otherwise the axis' values
		// left out.
		var topNames map[string]bool
		var topPlats map[string]bool
		if a.special == specialOther {
			if machine && len(a.chips) == 0 {
				topPlats = a.topPlatformKeys()
			} else {
				topNames = a.topNames()
			}
		}
		// A family is one card: whichever member matches the shelf, the menu or
		// the search is the same game, and the card is its base dump. The other
		// dumps are reached from the card's version button, not as cards of
		// their own.
		eligible := map[int]bool{}
		for _, g := range a.lib.Games {
			p := a.lib.Platform(g.PlatformKey)
			if p == nil || p.Group != a.group {
				continue
			}
			switch a.special {
			case specialFav:
				if !g.Favorite {
					continue
				}
			case specialOther:
				if machine && g.PlatformKey != a.platform && len(a.chips) > 0 {
					continue
				}
				if topPlats != nil {
					if topPlats[g.PlatformKey] {
						continue
					}
				} else if topNames != nil && topNames[g.Menu] {
					continue
				}
			default:
				if machine {
					if g.PlatformKey != a.platform {
						continue
					}
				} else if a.menu != "" && g.Menu != a.menu {
					continue
				}
			}
			if q != "" && !a.versionMatches(g, q) {
				continue
			}
			eligible[a.versionBase(g).ID] = true
		}
		for _, g := range a.lib.Games {
			if eligible[g.ID] {
				a.shown = append(a.shown, g)
			}
		}
	}
	a.file()
	a.chosen = 0
	a.scroll.Y = 0
}

// versionMatches reports whether the game or any dump of its family has the
// searched text, so looking for "真侍魂 X" still finds the one card. A rom's own
// name is searched too — "hsf2" or "mjelctrn" is what is written on the file
// that was copied in, before anyone typed a title for it.
func (a *App) versionMatches(g library.Game, query string) bool {
	matches := func(m library.Game) bool {
		if strings.Contains(strings.ToLower(m.Title), query) {
			return true
		}
		return strings.Contains(strings.ToLower(romFile(m.RomPath)), query)
	}
	if matches(g) {
		return true
	}
	for _, m := range a.verMembers[a.verFam[g.ID]] {
		if matches(m) {
			return true
		}
	}
	return false
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
	// 收藏 and 其他 mix several categories, so their headers name the machine (or
	// the arcade's board) instead of a category the bar never listed.
	if a.special != "" {
		a.fileByMachine()
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

// fileByMachine files 收藏 and 其他 by the hardware: a console's machine, a
// handheld's, an arcade game's 基板 — the names the card's own menu shows. The
// busiest hardware comes first, unknown last, and the cards inside it by name.
func (a *App) fileByMachine() {
	hw := func(g library.Game) string { return a.hardware(g) }
	counts := map[string]int{}
	for _, g := range a.shown {
		counts[hw(g)]++
	}
	sort.SliceStable(a.shown, func(i, j int) bool {
		mi, mj := hw(a.shown[i]), hw(a.shown[j])
		if mi != mj {
			if counts[mi] != counts[mj] {
				return counts[mi] > counts[mj]
			}
			return mi < mj
		}
		return titleLess(a.shown[i].Title, a.shown[j].Title)
	})
	for i := 0; i < len(a.shown); {
		j := i + 1
		for j < len(a.shown) && hw(a.shown[j]) == hw(a.shown[i]) {
			j++
		}
		a.sections = append(a.sections, section{name: hw(a.shown[i]), from: i, count: j - i})
		i = j
	}
}

// hardware names the machine a card belongs to on 收藏 and 其他: an arcade game
// or a mahjong table files under its 基板 once the board is known, and under
// its platform otherwise — on a console the platform is the machine, a
// handheld's machine comes from its core.
func (a *App) hardware(g library.Game) string {
	p := a.lib.Platform(g.PlatformKey)
	if p == nil {
		return "未知机种"
	}
	switch p.Group {
	case "handheld":
		return a.lib.MachineNameOf(g)
	case "arcade", "mahjong":
		if b := library.BoardNameOf(g); b != "未标注基板" {
			return b
		}
	}
	return p.Name
}

// topChips are the values the bar lists: the axis' first five.
func (a *App) topChips() []library.Chip {
	if len(a.chips) <= 5 {
		return a.chips
	}
	return a.chips[:5]
}

// topNames are those values, by name, so 其他 can hold the rest.
func (a *App) topNames() map[string]bool {
	top := a.topChips()
	out := make(map[string]bool, len(top))
	for _, c := range top {
		out[c.Name] = true
	}
	return out
}

// topPlatformKeys are the five machines the bar lists on a shelf whose machine
// is its platform: the ones 其他 leaves out.
func (a *App) topPlatformKeys() map[string]bool {
	top := a.topPlatforms(5)
	out := make(map[string]bool, len(top))
	for _, p := range top {
		out[p.Key] = true
	}
	return out
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

// versionOpen is the open flag of one card's version popup: a card owns its
// flag, the same way it owns its menu's.
func (a *App) versionOpen(id int) *bool {
	if a.verOpen == nil {
		a.verOpen = map[int]*bool{}
	}
	p, ok := a.verOpen[id]
	if !ok {
		p = new(bool)
		a.verOpen[id] = p
	}
	return p
}

// versionFor makes this card the one the open popup belongs to and closes the
// other cards' flags, so only one list of versions is ever on screen.
func (a *App) versionFor(id int) {
	if a.verGame == id {
		return
	}
	for k, p := range a.verOpen {
		if k != id {
			*p = false
		}
	}
	a.verGame = id
	a.verSel = 0
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

// vmenuOpen is the open flag of one version row's menu, made the first time the
// row is built: the row owns it, as a card owns its own.
func (a *App) vmenuOpen(id int) *bool {
	if a.vmenu == nil {
		a.vmenu = map[int]*bool{}
	}
	p, ok := a.vmenu[id]
	if !ok {
		p = new(bool)
		a.vmenu[id] = p
	}
	return p
}

// vmenuFor makes this row the one the open menu belongs to, closing the others.
func (a *App) vmenuFor(id int) {
	if a.vmenuGame == id {
		return
	}
	for k, p := range a.vmenu {
		if k != id {
			*p = false
		}
	}
	a.vmenuGame = id
}

// shutVMenu closes the version rows' menus.
func (a *App) shutVMenu() {
	for _, p := range a.vmenu {
		*p = false
	}
	a.vmenuGame = 0
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
	id, v := a.menuGame, strings.TrimSpace(value)
	if v == "" {
		return
	}
	// The menu is drawn from this frame's lines, so the write and the re-file
	// wait for the next frame; the menu closes now, as a click should.
	a.shutMenu()
	a.later(func() { a.writeField(id, field, v) })
}

// writeField is the work assign queued: the game's row, the library, and the
// body, all brought up to date before the next frame is laid out.
func (a *App) writeField(id int, field, value string) {
	v := value
	g := a.lib.Game(id)
	if g == nil || v == "" {
		return
	}
	var err error
	switch field {
	case "title":
		// 游戏名就是卡片上写的那一行：改名要把 name 列和内存里的 Title 一起换，
		// 这样下次启动读的还是改过的名字，而不是 rom 的文件名。
		g.Name, g.Title = v, v
		if a.db != nil {
			err = library.SetName(a.db, g.ID, v)
		}
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
	switch {
	case err != nil:
		a.status = fmt.Sprintf("%s 没改成：%v", name, err)
	case field == "title":
		a.status = "已改名为 " + v
	default:
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
}

// write is one column of one game into the database, when the launcher has
// one: a test builds an App without it.
func (a *App) write(id int, field, value string) error {
	if a.db == nil {
		return nil
	}
	return library.SetField(a.db, id, field, value)
}

// toggleFavorite stars the card the menu was opened on, or takes the star
// away. The write waits for the next frame like any other change of the body:
// taking a star away from a card on 收藏 removes it from that page.
func (a *App) toggleFavorite(id int) {
	g := a.lib.Game(id)
	if g == nil {
		return
	}
	on := !g.Favorite
	a.shutMenu()
	a.later(func() { a.setFavorite(id, on) })
}

// setFavorite is the work toggleFavorite queued: the row, the library, and the
// body, all up to date before the next frame is laid out.
func (a *App) setFavorite(id int, on bool) {
	g := a.lib.Game(id)
	if g == nil {
		return
	}
	g.Favorite = on
	var err error
	if a.db != nil {
		err = library.SetFavorite(a.db, id, on)
	}
	name := strings.ToUpper(g.Title)
	switch {
	case err != nil:
		g.Favorite = !on
		a.status = fmt.Sprintf("%s 没能收藏：%v", name, err)
		return
	case on:
		a.status = name + " 已加入收藏"
	default:
		a.status = name + " 已取消收藏"
	}
	at := a.scroll.Y
	a.recount()
	a.refilter()
	a.scroll.Y = at
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
	if a.platform == key && a.special == "" {
		return
	}
	a.platform, a.menu, a.query, a.special = key, "", "", ""
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
	a.axis, a.menu, a.moreOpen, a.special = axis, "", false, ""
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
	a.group, a.menu, a.query, a.special = group, "", "", ""
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
	a.platformTotal, a.shelfTotal, a.favTotal, a.otherTotal = 0, 0, 0, 0
	if a.lib == nil {
		return
	}
	for _, g := range a.lib.Games {
		p := a.lib.Platform(g.PlatformKey)
		if p == nil || p.Group != a.group {
			continue
		}
		a.shelfTotal++
		if g.Favorite {
			a.favTotal++
		}
		if g.PlatformKey == a.platform {
			a.platformTotal++
		}
		if g.Menu != "" {
			a.menuCounts[g.Menu]++
		}
	}
	a.chips = a.lib.Chips(a.group, a.axis)
	// 其他 is everything the bar's five chips do not hold — the axis' values left
	// out, or, on a shelf whose machine is its platform, the machines left out.
	top := 0
	if (a.axis == library.AxisMachine || a.axis == "") && len(a.chips) == 0 {
		for _, p := range a.topPlatforms(5) {
			top += a.counts[p.Key]
		}
	} else {
		for _, c := range a.topChips() {
			top += c.Count
		}
	}
	a.otherTotal = max(0, a.shelfTotal-top)
}

func (a *App) chooseMenu(name string) {
	if a.special == "" && a.menu == name {
		return
	}
	a.special, a.menu = "", name
	a.refilter()
}

// chooseSpecial opens one of the bar's two pages: 收藏, the shelf's starred
// cards, or 其他, the categories the five chips left out. A click on the page
// already showing one closes it again — on a shelf whose chips are its
// machines there is no 全部 to go back to.
func (a *App) chooseSpecial(kind string) {
	if a.special == kind {
		kind = ""
	}
	a.special, a.menu = kind, ""
	a.moreOpen, a.shelfOpen = false, false
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
	if a.special == "" && (a.axis == library.AxisMachine || a.axis == "") {
		return a.platformName()
	}
	name := ""
	for _, g := range library.Groups {
		if g.Key == a.group {
			name = g.Name
		}
	}
	value := a.menu
	switch {
	case a.special == specialFav:
		value = "收藏"
	case a.special == specialOther:
		value = "其他"
	case value == "":
		value = "全部"
	}
	return name + " · " + value
}

// launchGame starts the chosen game. The emulation happens in this process:
// the core that RetroArch ships as a library is loaded here, the app steps it,
// and the picture is drawn into this window's page. There is no second window.
// No core, whatever it asks of the frontend, is ever started as another program:
// a game that this window cannot run is reported here instead.
func (a *App) launchGame(g library.Game) {
	if a.lib == nil || a.core != nil {
		return
	}
	// The card of a family plays the version chosen under it; a card that is
	// itself a version plays itself.
	g = a.versionTarget(g)
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
	// DuckStation draws PlayStation with OpenGL and dies inside this process on
	// its first context; Mednafen's PSX core draws the same discs in software,
	// so a PlayStation game is stepped with that one here. The saved sim is
	// left as it was, so the same choice still works if the core is fixed.
	if strings.EqualFold(sim.Core, "duckstation") {
		s := *sim
		s.Core = "mednafen_psx"
		sim = &s
	}
	// MAME's libretro core takes the rom's folder as a "system" it must know,
	// and reads its own files through the ANSI code page, which the emulator's
	// Chinese folder breaks; the ASCII system mirror beside it answers that. A
	// core that draws with OpenGL is given a context inside this process. Either
	// way the game is stepped here, in this window, with no second program.

	rom := filepath.Join(a.root, "rom", filepath.FromSlash(g.RomPath))
	exe := sim.Exe
	if !filepath.IsAbs(exe) {
		exe = filepath.Join(a.root, filepath.FromSlash(exe))
	}
	dir := filepath.Dir(exe) // the core and the system directory sit beside the emulator
	corePath := core.CorePath(dir, sim.Core)
	if corePath == "" {
		a.status = fmt.Sprintf("%s：找不到核心 %s", g.Title, sim.Core)
		return
	}
	info, err := os.Stat(rom)
	if err != nil {
		a.status = "启动失败：" + err.Error()
		return
	}
	if info.IsDir() {
		a.status = "启动失败：rom 不是文件"
		return
	}
	// IPS 补丁：这个 rom 打开的补丁组会套到它的一份副本上，rom/ 里的文件一个字节都
	// 不动。副本按（rom + 补丁组）落在缓存里，同一套补丁只构建一次。
	//
	// 副本怎么交给核心分两条路。FBNeo 拿着打过补丁的 zip 当 rom 会被它按校验和退回
	// （"ROM … is missing"），但它自己会去 <system>\fbneo\patched\<set>.zip 找一份
	// 覆盖用的 set，那份放过校验、只查名字和大小：所以对 FBNeo 是把副本放进去、原
	// rom 照给，并告诉它允许打过补丁的 set；别的核心（MAME）没有这个目录，直接吃副本。
	systemDir := a.asciiSystem(dir)
	playRom := rom
	patches := a.ipsEnabled(g)
	var options []string
	if len(patches) > 0 {
		patched, err := ips.Prepare(rom, patches, filepath.Join(a.root, "模拟器", "RetroArch", "cache", "ips"))
		if err != nil {
			a.status = "ips 补丁失败：" + err.Error()
			return
		}
		if isFBNeoCore(sim.Core) {
			if err := stagePatchedSet(systemDir, rom, patched); err != nil {
				a.status = "ips 补丁失败：" + err.Error()
				return
			}
			options = append(options, "fbneo-allow-patched-romsets=enabled")
		} else {
			playRom = patched
		}
	} else {
		// 这一份没开补丁：上一次留下的同名 patched set 不能盖过它。
		clearPatchedSet(systemDir, rom)
	}
	// The core is loaded and stepped on one thread of its own. Creating the
	// OpenGL context on the window's thread and running the core on another is
	// what left the Dreamcast core drawing into a framebuffer nobody read: the
	// numbers meant different objects in the two contexts. One thread owns the
	// context, the load, the run loop and the readback.
	type loadedCore struct {
		sess *core.Session
		err  error
	}
	loadedCh := make(chan loadedCore, 1)
	runGate := make(chan struct{})
	go func() {
		runtime.LockOSThread()
		sess, err := core.Start(corePath, playRom, systemDir, options...)
		loadedCh <- loadedCore{sess, err}
		if err != nil {
			return
		}
		// Every frame the core has drawn is also a chance to file it in the row
		// the timeline steps back through. The capture runs on the core's own
		// thread, right after the frame, which is the one moment its state is whole.
		sess.OnFrame(func() {
			a.captureRewind(sess)
			a.padLate(sess)
			a.win.Update(func() {})
		})
		<-runGate
		sess.Run()
		a.win.Update(func() {
			if a.core != sess {
				return // a newer game, or the user, already took the window back
			}
			a.releaseGame()
			launch.SetWindowTitle(a.win.NativeHandle(), "GameRoom")
			a.status = g.Title + " 已退出"
		})
	}()
	lc := <-loadedCh
	if lc.err != nil {
		a.status = "启动失败：" + lc.err.Error()
		return
	}
	sess := lc.sess
	a.core = sess
	a.gameBmp = nil
	a.gameFrames = 0
	a.gameAspect = sess.Aspect()
	a.playerGame = g
	a.padsOpen = false
	a.padLive = a.padLayoutOf(g)
	// The core's own names for the machine's buttons decide how many the panel
	// draws and which id each of them presses: FBNeo reads its Button 1 from B
	// and its Button 2 from A, a Street Fighter cabinet puts the punches on
	// Y/X/L, and a six-button game on a three-button board still gets six
	// buttons. Read here, from the core itself, once per game.
	a.padNow = padPanelFor(padLayoutFor(a.padLive), sess.InputDescs())
	a.padIDs = padIDsFor(a.padNow, sess.InputDescs())
	// FBNeo announces its buttons after the first frame rather than during load, so
	// the read above can come back empty there; the frame hook asks again (padLate).
	a.padGame, a.padCore = g, corePath
	a.padIDsDone = len(sess.InputDescs()) > 0
	// What the launcher believes the core's buttons are goes into 数据/按键自检.txt,
	// where a wrong button can be read off instead of guessed at.
	a.padCheckStart(g, corePath, a.padNow, a.padProfile(a.padLive), sess.InputDescs(), a.padIDs)
	sess.SetKeyBinds(0, a.keyBinds(a.padLive))
	sess.SetKeyBinds(1, a.keyBinds2(a.padLive))
	a.filtersOpen = false
	a.cap = ""
	a.hold = [2]padHold{}
	// A new game starts with an empty shelf view and an empty row: the slots are
	// read when the panel opens, and the row is planned from this core's own state.
	a.savesOpen = false
	a.slots, a.shots, a.slotSel, a.slotNote = nil, nil, 0, ""
	a.scrub = scrubDrag{}
	a.rew.reset()
	// The core's sound plays through the machine's own wave output, queued a
	// few frames ahead: the game is the only thing making noise.
	sess.StartSound()
	if os.Getenv("GAMEROOM_DEBUG") != "" {
		pipe := sess.Audio()
		go func() {
			for i := 0; i < 5; i++ {
				time.Sleep(2 * time.Second)
				fmt.Fprintf(os.Stderr, "sound: pushed=%d reads=%d loudest=%.3f rate=%d\n", pipe.Pushed(), pipe.Reads(), pipe.Loudest(), pipe.SampleRate())
			}
		}()
	}
	// The window is the game's window now: its caption carries the game — and,
	// on the right, how many ips patch groups it is running with — and the page
	// carries only the picture and the two controls over it.
	launch.SetWindowTitle(a.win.NativeHandle(), ipsTitle(g.Title, len(patches)))
	a.status = g.Title
	// Everything the window needs from the session is in place, so the thread
	// that owns the context may start stepping the game.
	close(runGate)
}

// stopPlayer ends the game on screen and gives the library its window back.
func (a *App) stopPlayer() {
	if a.core != nil {
		s := a.core
		a.core = nil
		s.Close()
		a.releaseGame()
		launch.SetWindowTitle(a.win.NativeHandle(), "GameRoom")
		a.status = ""
		return
	}
	if a.player == nil {
		if a.external != nil {
			// The game in the external program: closing it is killing it, and the
			// launcher's picture comes back when its process is gone.
			a.external.Process.Kill()
			return
		}
		return
	}
	a.player.Kill()
}

// releaseGame forgets the game that was on screen: its picture, its sound and
// the title that named it. The session itself is closed by the caller.
func (a *App) releaseGame() {
	a.gameBmp = nil
	a.gameFrames = 0
	a.frameW, a.frameH = 0, 0
	a.gameAspect = 0
	a.mouseSeen = false
	a.playerGame = library.Game{}
	a.filtersOpen = false
	a.padsOpen = false
	a.padLive = ""
	a.padIDs = nil
	a.padNow = nil
	a.cap = ""
	a.savesOpen = false
	a.slots, a.shots, a.slotSel, a.slotNote = nil, nil, 0, ""
	a.scrub = scrubDrag{}
	a.rew.reset()
}

// setFilter records a look and applies it to the running game. RetroArch reads
// a preset when it loads one, so the app writes the preset for the look and
// hands it to the running instance over its network interface — the game keeps
// playing, its state and its place on screen untouched. Only a RetroArch that
// does not answer has to be restarted to take the new look.
func (a *App) setFilter(level string) {
	if level == a.cfg.FilterLevel {
		a.filtersOpen = false
		if a.player != nil {
			a.player.Show()
		}
		return
	}
	prev := a.cfg.FilterLevel
	a.cfg.FilterLevel = level
	a.saveConfig()
	a.filtersOpen = false
	if a.core != nil {
		// The look is the app's own now, drawn by this process over the picture;
		// it is kept for the next game rather than bent into a running core.
		a.status = a.playerGame.Title + " · " + filterName(level)
		return
	}
	if a.player == nil {
		return // nothing running: the next game starts with it
	}
	a.player.Show()
	var err error
	if level == "off" {
		// A preset with no passes takes the whole pipeline down, so the filter
		// is switched off rather than replaced — the game keeps its frame.
		if prev != "off" {
			err = launch.SendCommand(launch.CommandPort, "SHADER_TOGGLE")
		}
	} else {
		var path string
		path, err = launch.WriteLivePreset(a.playerTarget, a.cfg.CRTShader, level)
		if err == nil {
			err = launch.SetShaderLive(launch.CommandPort, path)
		}
	}
	if err == nil {
		a.status = launch.EmbedStatus(a.playerGame.Title) + " · " + filterName(level)
		return
	}
	// The interface is off or unanswered: this instance can only change look by
	// coming up again.
	g := a.playerGame
	a.status = "滤镜像已保存，重启游戏生效"
	a.player.Kill()
	time.AfterFunc(700*time.Millisecond, func() {
		a.win.Update(func() {
			if a.player != nil {
				return // the old game is still going; its exit starts nothing
			}
			a.launchGame(g)
		})
	})
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
	a.imgMu.Lock()
	defer a.imgMu.Unlock()
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
	a.pumpImagesLocked()
	return nil
}

// pumpImages decodes the queue strictly one picture at a time: scrolling fast
// must not open a decoder per visible card. Each finished picture asks for the
// next, so the order the grid asked in is the order they arrive.
func (a *App) pumpImages() {
	a.imgMu.Lock()
	defer a.imgMu.Unlock()
	a.pumpImagesLocked()
}

func (a *App) pumpImagesLocked() {
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
			a.imgMu.Lock()
			if job.icon {
				a.icons[job.path] = b
			} else {
				a.covers[job.path] = b
			}
			a.loading = false
			a.pumpImagesLocked()
			a.imgMu.Unlock()
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
