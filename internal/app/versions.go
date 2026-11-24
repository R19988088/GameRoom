package app

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/R19988088/GameRoom/internal/library"
)

// The version picker: one game of a series is the card, and the other dumps of
// the same game — a 改版, a 特别版, another region — hang under it as versions
// with their language. The card keeps a small button that says how many there
// are; the popup lists them and a double click makes one the one that plays.

// versionPrefix is the key a version default is stored under, per family.
const versionPrefix = "ver:"

// versionTags are the endings a title carries when it names a version rather
// than another game: a dump's own suffix, or the language it was dumped in. They
// are stripped before two titles are compared, so "霸王丸地狱变" and
// "霸王丸地狱变 sp" are the same game.
var versionTags = []string{
	"sp", "special", "plus", "turbo", "ae", "ex", "x", "ver",
	"特别版", "加强版", "增强版", "中文版", "汉化版", "日版", "美版",
	"欧版", "世界版", "国际版", "亚洲版", "无作弊版", "奖分版", "改版",
	"盗版", "冠军版", "终极版", "终级版", "完全版", "合集",
}

// latinNumerals are the Roman numerals as one rune, so Ⅱ and II compare the
// same: a shelf mixes both spellings.
var latinNumerals = map[rune]string{
	'Ⅰ': "i", 'Ⅱ': "ii", 'Ⅲ': "iii", 'Ⅳ': "iv", 'Ⅴ': "v", 'Ⅵ': "vi",
	'Ⅶ': "vii", 'Ⅷ': "viii", 'Ⅸ': "ix", 'Ⅹ': "x", 'Ⅺ': "xi", 'Ⅻ': "xii",
}

// normTitle is a title in the shape two versions of a game agree on: lower case,
// Roman numerals as letters, parenthetical notes and version tags gone.
func normTitle(title string) string {
	var b strings.Builder
	for _, r := range strings.TrimSpace(title) {
		if v, ok := latinNumerals[r]; ok {
			b.WriteString(v)
			continue
		}
		switch r {
		case '（':
			b.WriteRune('(')
		case '）':
			b.WriteRune(')')
		case '　':
			b.WriteRune(' ')
		default:
			b.WriteRune(r)
		}
	}
	s := strings.ToLower(b.String())
	// A note in brackets is not the game ("(日版, 无作弊版)"), and a title can
	// carry more than one.
	for {
		trimmed := strings.TrimSpace(s)
		if strings.HasSuffix(trimmed, "]") {
			if i := strings.LastIndex(trimmed, "["); i >= 0 {
				s = strings.TrimSpace(trimmed[:i])
				continue
			}
		}
		if strings.HasSuffix(trimmed, ")") {
			if i := strings.LastIndex(trimmed, "("); i >= 0 {
				s = strings.TrimSpace(trimmed[:i])
				continue
			}
		}
		break
	}
	// Tags come off the end one at a time: "特别版 sp" is still 特别版.
	for changed := true; changed; {
		changed = false
		s = strings.TrimSpace(s)
		for _, tag := range versionTags {
			if strings.HasSuffix(s, " "+tag) && len(s) > len(tag)+1 {
				s = strings.TrimSpace(s[:len(s)-len(tag)-1])
				changed = true
				break
			}
		}
	}
	return strings.TrimSpace(s)
}

// sequelRemainder reports whether what follows a base title names another game
// rather than a version: "2", "III", "4 进化", or an English word of its own
// ("Kids"). A sequel is its own game and must not be folded under an earlier
// one; a version's tail is Chinese, or one of the tags stripped above.
func sequelRemainder(rest string) bool {
	rest = strings.TrimSpace(rest)
	if rest == "" {
		return true
	}
	first := strings.ToLower(strings.Fields(rest)[0])
	switch first {
	case "i", "ii", "iii", "iv", "v", "vi", "vii", "viii", "ix", "x", "xi", "xii":
		return true
	}
	if first[0] >= '0' && first[0] <= '9' {
		return true
	}
	if strings.HasPrefix(first, "第") {
		return true
	}
	// An English tail that is not one of the version words is a name of its
	// own: "Virtua Fighter Kids" is not a version of "Virtua Fighter".
	asciiWord := true
	for _, r := range first {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') {
			asciiWord = false
			break
		}
	}
	if asciiWord {
		isTag := false
		for _, tag := range versionTags {
			if first == tag {
				isTag = true
				break
			}
		}
		return !isTag
	}
	return false
}

// versionKeyOf is the family a game belongs to inside its platform: the
// shortest title of the series, or its own when it stands alone.
func versionKeyOf(g library.Game, all []library.Game, norm map[int]string) string {
	n := norm[g.ID]
	key := n
	for i := range all {
		h := all[i]
		if h.ID == g.ID || h.PlatformKey != g.PlatformKey {
			continue
		}
		hn := norm[h.ID]
		if len(hn) > len(key) || len(hn) < 4 || !strings.HasPrefix(n, hn) {
			continue
		}
		if len(n) > len(hn) && n[len(hn)] != ' ' {
			continue
		}
		if sequelRemainder(n[len(hn):]) {
			continue
		}
		key = hn
	}
	return key
}

// buildVersions files every game under its family and reads the defaults the
// user has picked, so the cards and the popup agree from the first frame.
func (a *App) buildVersions() {
	a.verFam = map[int]string{}
	a.verMembers = map[string][]library.Game{}
	a.verDefault = map[string]int{}
	if a.lib == nil {
		return
	}
	norm := make(map[int]string, len(a.lib.Games))
	for _, g := range a.lib.Games {
		n := normTitle(g.Title)
		if n == "" {
			n = fmt.Sprintf("#%d", g.ID)
		}
		norm[g.ID] = n
	}
	for _, g := range a.lib.Games {
		key := g.PlatformKey + "|" + versionKeyOf(g, a.lib.Games, norm)
		a.verFam[g.ID] = key
		a.verMembers[key] = append(a.verMembers[key], g)
	}
	// Two games that merely share a title ("无敌版" is a label, not a series)
	// are not versions of one another: a family exists only when one title is
	// the base and another spells it out at greater length.
	for key, members := range a.verMembers {
		base := strings.TrimPrefix(key, members[0].PlatformKey+"|")
		longer := false
		distinct := false
		for i, m := range members {
			if len(norm[m.ID]) > len(base) {
				longer = true
			}
			if i > 0 && m.Title != members[0].Title {
				distinct = true
			}
		}
		// A family is a base title plus at least one title that spells it out
		// longer ("霸王丸地狱变"), or the same title written differently
		// ("(日版, 无作弊版)" vs "sp") — but only for a real title: two dumps
		// that are both called "无敌版" are different games with the same label.
		if longer || (distinct && len(base) >= 4) {
			continue
		}
		for _, m := range members {
			id := fmt.Sprintf("%s#%d", key, m.ID)
			a.verFam[m.ID] = id
			a.verMembers[id] = []library.Game{m}
		}
		delete(a.verMembers, key)
	}
	// What the user filed by hand wins over the titles: a 改版 dropped onto a
	// card is shown under it, and a dump marked 取消合并 keeps a card of its own.
	// The repointing happens after the title pass, so an explicit group is never
	// split again by the rule above.
	if a.hasFiling() {
		keys := make(map[int]string, len(a.lib.Games))
		for id, k := range a.verFam {
			keys[id] = k
		}
		byID := make(map[int]library.Game, len(a.lib.Games))
		for _, g := range a.lib.Games {
			byID[g.ID] = g
		}
		for _, g := range a.lib.Games {
			switch {
			case g.Solo:
				keys[g.ID] = fmt.Sprintf("%s#%d", g.PlatformKey, g.ID)
			case g.Family != 0 && g.Family != g.ID:
				// A card of the same shelf only: a note that outlived the game it
				// pointed at files nothing.
				if b, ok := byID[g.Family]; ok && b.PlatformKey == g.PlatformKey {
					if k, ok := keys[g.Family]; ok {
						keys[g.ID] = k
					}
				}
			}
		}
		a.verMembers = map[string][]library.Game{}
		for _, g := range a.lib.Games {
			k := keys[g.ID]
			a.verFam[g.ID] = k
			a.verMembers[k] = append(a.verMembers[k], g)
		}
	}
	for key := range a.verMembers {
		if a.db == nil {
			break
		}
		if v := library.Setting(a.db, versionPrefix+key); v != "" {
			if id, err := strconv.Atoi(v); err == nil {
				a.verDefault[key] = id
			}
		}
	}
}

// hasFiling reports whether any dump was filed by hand: without one, the titles
// alone decide the families and there is nothing to repoint.
func (a *App) hasFiling() bool {
	if a.lib == nil {
		return false
	}
	for _, g := range a.lib.Games {
		if g.Solo || g.Family != 0 {
			return true
		}
	}
	return false
}

// versionsOf returns the versions of a game's family, the base dump first and
// the rest by title. It is nil for a game that has no other version.
func (a *App) versionsOf(g library.Game) []library.Game {
	members := a.verMembers[a.verFam[g.ID]]
	if len(members) < 2 {
		return nil
	}
	base := a.versionBase(g)
	out := make([]library.Game, 0, len(members))
	for _, m := range members {
		if m.ID == base.ID {
			out = append([]library.Game{m}, out...)
		} else {
			out = append(out, m)
		}
	}
	sort.SliceStable(out[1:], func(i, j int) bool { return out[1+i].Title < out[1+j].Title })
	return out
}

// versionBase is the dump a family is filed under: the title most of its
// members share (the original dump and its revisions), and the shortest
// spelling of it — not the hack whose name happens to be shorter.
func (a *App) versionBase(g library.Game) library.Game {
	return baseOf(a.verMembers[a.verFam[g.ID]], g)
}

// baseOf is versionBase over the given members, for picking a base among the
// rest of a family while one of them is being taken out of it.
func baseOf(members []library.Game, fallback library.Game) library.Game {
	counts := map[string]int{}
	best, bestCount := "", 0
	for _, m := range members {
		n := normTitle(m.Title)
		counts[n]++
		if counts[n] > bestCount || (counts[n] == bestCount && len(n) > len(best)) {
			best, bestCount = n, counts[n]
		}
	}
	var base library.Game
	found := false
	for _, m := range members {
		if normTitle(m.Title) != best {
			continue
		}
		if !found || len(m.Title) < len(base.Title) ||
			(len(m.Title) == len(base.Title) && m.ID < base.ID) {
			base, found = m, true
		}
	}
	if !found {
		return fallback
	}
	return base
}

// versionDefault is the dump that plays when the family's card is opened; 0
// when the user has not chosen one.
func (a *App) versionDefault(g library.Game) int {
	if id := a.verDefault[a.verFam[g.ID]]; id != 0 {
		return id
	}
	return 0
}

// setVersionDefault makes a dump the one that plays for its family.
func (a *App) setVersionDefault(g library.Game, id int) {
	key := a.verFam[g.ID]
	// 库没打开（db 为 nil）时只记在内存里：SetSetting 收 nil db 会直接崩。
	if a.db != nil {
		if err := library.SetSetting(a.db, versionPrefix+key, strconv.Itoa(id)); err != nil {
			a.status = "默认版本没存上：" + err.Error()
			return
		}
	}
	a.verDefault[key] = id
	for _, m := range a.verMembers[key] {
		if m.ID == id {
			a.status = "默认版本：" + m.Title
			return
		}
	}
}

// versionTarget is the game a card actually starts: the family's base card
// plays the dump chosen as default, while a card that is itself a version plays
// that version.
func (a *App) versionTarget(g library.Game) library.Game {
	if len(a.verMembers[a.verFam[g.ID]]) < 2 {
		return g
	}
	if base := a.versionBase(g); base.ID != g.ID {
		return g
	}
	id := a.versionDefault(g)
	if id == 0 || id == g.ID {
		return g
	}
	for _, m := range a.verMembers[a.verFam[g.ID]] {
		if m.ID == id {
			return m
		}
	}
	return g
}

// versionLabel is what a version is called under the base title on the card's
// button: the dump's own tail ("霸王丸地狱变 sp"), or the whole title when the
// two titles do not share a base. The list under the button shows versionName
// instead — the dump's whole title.
func (a *App) versionLabel(base, g library.Game) string {
	label := ""
	if g.ID != base.ID && strings.HasPrefix(g.Title, base.Title) {
		label = strings.TrimSpace(g.Title[len(base.Title):])
	}
	if label == "" {
		label = g.Title
	}
	// Two dumps can carry the same title ("麻将 大中华圈" twice); the rom file
	// is what tells them apart, so it is shown rather than a duplicate row.
	if a.sameTitle(g) {
		label += " · " + a.romTag(g)
	}
	return label
}

// versionName is what a version is called in the list under the button: its
// whole title — the base dump is not a "主 ROM" line, it is the game itself — and
// the rom's name after it when two dumps share a title.
func (a *App) versionName(g library.Game) string {
	name := g.Title
	if a.sameTitle(g) {
		name += " · " + a.romTag(g)
	}
	return name
}

// romTag is what tells two dumps of the same game apart in a list: their rom's
// name — and the whole file name, extension included, when the same name was
// dumped into two containers, since "hsf2" twice would be two identical rows.
func (a *App) romTag(g library.Game) string {
	stem := romStem(g.RomPath)
	for _, m := range a.verMembers[a.verFam[g.ID]] {
		if m.ID != g.ID && strings.EqualFold(romStem(m.RomPath), stem) {
			return romFile(g.RomPath)
		}
	}
	return stem
}

// sameTitle tells whether another dump of the family carries this dump's title.
func (a *App) sameTitle(g library.Game) bool {
	same := 0
	for _, m := range a.verMembers[a.verFam[g.ID]] {
		if m.Title == g.Title {
			same++
		}
	}
	return same > 1
}

// romStem is a rom's file name without its extension, as "samsho2".
func romStem(path string) string {
	return strings.TrimSuffix(romFile(path), extOf(path))
}

// romFile is a rom's file name with its extension, as "hsf2.7z": two dumps of
// one game in two containers differ only here.
func romFile(path string) string {
	base := path
	if i := strings.LastIndexAny(base, `/\`); i >= 0 {
		base = base[i+1:]
	}
	return base
}

// extOf is the extension of a path as ".7z", empty when it has none.
func extOf(path string) string {
	base := romFile(path)
	if i := strings.LastIndex(base, "."); i > 0 {
		return base[i:]
	}
	return ""
}

// gameByFile is the game of a platform whose rom is the dropped file. A dump
// dropped from Explorer is the same file the library already points at, so its
// name, or its name without the extension, finds the row.
func (a *App) gameByFile(platform, name string) int {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return 0
	}
	stem := romStem(name)
	for _, g := range a.lib.Games {
		if g.PlatformKey != platform {
			continue
		}
		rom := strings.ToLower(filepath.Base(strings.ReplaceAll(g.RomPath, `\`, "/")))
		if rom == name || romStem(rom) == stem {
			return g.ID
		}
	}
	return 0
}

// mergeDumps files the dropped files under the card they were dropped on: a
// 改版 dragged onto another title stops being a card of its own. Only the
// library is written; a rom stays in the folder it is in.
func (a *App) mergeDumps(target library.Game, paths []string) {
	a.shutMenu()
	a.later(func() { a.mergeFiles(target.ID, paths) })
}

// expandPaths turns a drop into the files to look up. A dropped folder stands
// for the roms in it, which is the "choose the folder" way of filing a pile of
// hacks at once: Explorer hands over the folder, the library matches the roms
// inside it by name.
func (a *App) expandPaths(paths []string, p *library.Platform) []string {
	exts := map[string]bool{".zip": true, ".7z": true}
	if p != nil && len(p.Exts) > 0 {
		exts = map[string]bool{}
		for _, e := range p.Exts {
			exts[strings.ToLower("."+strings.TrimLeft(e, "."))] = true
		}
	}
	out := make([]string, 0, len(paths))
	for _, path := range paths {
		st, err := os.Stat(path)
		if err != nil || st.IsDir() == false {
			out = append(out, path)
			continue
		}
		entries, err := os.ReadDir(path)
		if err != nil {
			out = append(out, path)
			continue
		}
		for _, e := range entries {
			if e.IsDir() || !exts[strings.ToLower(filepath.Ext(e.Name()))] {
				continue
			}
			out = append(out, filepath.Join(path, e.Name()))
		}
	}
	return out
}

// mergeFiles is the work mergeDumps queued: each dropped file the library knows
// is filed under the card's title. Files it does not know are counted, not
// imported — the import pass is where a rom first comes into the library.
func (a *App) mergeFiles(targetID int, paths []string) {
	base := a.lib.Game(targetID)
	if base == nil {
		return
	}
	baseID := a.versionBase(*base).ID
	paths = a.expandPaths(paths, a.lib.Platform(base.PlatformKey))
	merged, unknown := 0, 0
	for _, p := range paths {
		id := a.gameByFile(base.PlatformKey, filepath.Base(p))
		if id == 0 || id == baseID {
			unknown++
			continue
		}
		g := a.lib.Game(id)
		if g == nil {
			unknown++
			continue
		}
		var err error
		if a.db != nil {
			err = library.FileDump(a.db, id, baseID, false)
		}
		if err != nil {
			a.status = "没并上：" + err.Error()
			return
		}
		g.Family, g.Solo = baseID, false
		merged++
	}
	a.buildVersions()
	at := a.scroll.Y
	a.recount()
	a.refilter()
	a.scroll.Y = at
	switch {
	case merged > 0 && unknown > 0:
		a.status = fmt.Sprintf("已并入 %d 个改版到《%s》，%d 个文件不在库里",
			merged, base.Title, unknown)
	case merged > 0:
		a.status = fmt.Sprintf("已并入 %d 个改版到《%s》", merged, base.Title)
	case unknown > 0:
		a.status = fmt.Sprintf("这 %d 个文件不在库里", unknown)
	}
}

// cardDrag is what a card carries while the pointer drags it with Ctrl held:
// the game it was, for whichever card it is let go on to file it under.
type cardDrag struct{ id int }

// mergeCards files one card's game under another card, as a dump dropped on the
// card is filed: Ctrl-dragging a card onto another title makes it one of that
// card's versions, whatever its own name says.
func (a *App) mergeCards(from, onto int) {
	a.shutMenu()
	a.later(func() { a.mergeInto(from, onto) })
}

// mergeInto is the work mergeCards queued. The dragged game becomes a dump of
// the card it was let go on — the card's base, so a family never nests in
// another family — and the card it came from stops being a card of its own.
func (a *App) mergeInto(from, onto int) {
	src, dst := a.lib.Game(from), a.lib.Game(onto)
	if src == nil || dst == nil || src.ID == dst.ID {
		return
	}
	if src.PlatformKey != dst.PlatformKey {
		a.status = "只能并到同一个平台的游戏里"
		return
	}
	base := a.versionBase(*dst)
	if base.ID == src.ID || a.verFam[src.ID] == a.verFam[dst.ID] {
		a.status = fmt.Sprintf("《%s》已经在这张卡上了", src.Title)
		return
	}
	if a.db != nil {
		if err := library.FileDump(a.db, src.ID, base.ID, false); err != nil {
			a.status = "没并上：" + err.Error()
			return
		}
	}
	src.Family, src.Solo = base.ID, false
	a.buildVersions()
	at := a.scroll.Y
	a.recount()
	a.refilter()
	a.scroll.Y = at
	a.status = fmt.Sprintf("已把《%s》并到《%s》", src.Title, base.Title)
}

// splitDump takes a dump out of its family: 取消合并 gives it a card of its own,
// whatever its title says. Only the dump clicked comes apart; the rest of the
// family stays together.
func (a *App) splitDump(id int) {
	a.shutMenu()
	a.shutVMenu()
	a.later(func() { a.setSolo(id) })
}

// setSolo is the work splitDump queued. The dumps that pointed at the one going
// out have to point at whoever is left: a family's members are filed under a
// base by id, so splitting the base while they still said "跟着 3 号" would file
// them all back under it and nothing would come apart.
func (a *App) setSolo(id int) {
	g := a.lib.Game(id)
	if g == nil {
		return
	}
	var remaining []library.Game
	for _, m := range a.verMembers[a.verFam[g.ID]] {
		if m.ID != id {
			remaining = append(remaining, m)
		}
	}
	var err error
	if a.db != nil {
		err = library.FileDump(a.db, id, 0, true)
	}
	if err != nil {
		a.status = "没能独立：" + err.Error()
		return
	}
	g.Family, g.Solo = 0, true
	if len(remaining) > 0 {
		base := baseOf(remaining, remaining[0])
		for _, m := range remaining {
			mg := a.lib.Game(m.ID)
			if mg == nil || mg.Solo || mg.Family != id {
				continue
			}
			if a.db != nil {
				if err := library.FileDump(a.db, mg.ID, base.ID, false); err != nil {
					a.status = "没能独立：" + err.Error()
					return
				}
			}
			mg.Family = base.ID
		}
	}
	a.buildVersions()
	at := a.scroll.Y
	a.recount()
	a.refilter()
	a.scroll.Y = at
	a.status = g.Title + " 已取消合并"
}

// versionBar is the button a card shows under its title when the game has more
// than one dump: how many there are, and a dot when another dump than this card
// is the one that plays. A game without versions keeps the row empty, so every
// card's lower edge stays on the same line.
func (a *App) versionBar(c *ui.Context, t *ui.Theme, g library.Game) {
	versions := a.versionsOf(g)
	if len(versions) == 0 {
		ui.Box(c).Height(19)
		return
	}
	open := a.versionOpen(g.ID)
	def := a.versionDefault(g)
	label := fmt.Sprintf("版本 %d", len(versions))
	if def != 0 && def != g.ID {
		for _, m := range versions {
			if m.ID == def {
				label += " · " + a.versionLabel(a.versionBase(g), m)
				break
			}
		}
	}
	taken := def != 0 && def != g.ID

	pill := ui.ButtonBase(c).Height(19).PaddingX(8).Radius(9).
		AlignItems(ui.Center).Gap(5)
	pill.Background(ui.Hex("#ffffff0e")).Border(1, ui.Hex("#ffffff1f"))
	if *open {
		pill.Background(ui.Hex("#e8763a1f")).Border(1, ui.Hex("#e8763a80"))
	} else if pill.Hovered() {
		pill.Background(ui.Hex("#ffffff1a")).Border(1, ui.Hex("#ffffff33"))
	}
	pill.Children(func() {
		ui.Text(c, label).FontSize(10).
			FontWeight(600).TextColor(t.TextMuted.Alpha(0.92)).SingleLine()
		if taken {
			ui.Text(c, "●").FontSize(6).TextColor(t.Accent)
		}
		ui.Text(c, "▾").FontSize(9).TextColor(t.TextMuted.Alpha(0.6))
	})
	if pill.Clicked() {
		a.versionFor(g.ID)
		a.verSel = a.versionDefault(g)
		if a.verSel == 0 {
			a.verSel = versions[0].ID
		}
		*open = !*open
	}

	ui.PopoverBase(c, pill, open, func(panel ui.Element) {
		a.versionPanel(c, t, panel, g, versions)
	})
}

// versionPanel is the list under the button: one row per dump, its language, and
// a tick on the one that plays. A single click on a row makes it the default —
// the card plays it and shows its cover — and closes the list.
func (a *App) versionPanel(c *ui.Context, t *ui.Theme, panel ui.Element, g library.Game, versions []library.Game) {
	def := a.versionDefault(g)

	panel.Width(302).Padding(7).Radius(16).
		Background(ui.Hex("#17181ff5")).
		Border(1, ui.Hex("#ffffff1f")).
		Shadow(0, 22, 48, 0, ui.Hex("#000000b8"))
	panel.Children(func() {
		ui.Column(c).Gap(2).Children(func() {
			ui.Row(c).Height(32).PaddingX(10).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "版本").FontSize(12.5).FontWeight(700).
					TextColor(t.Text).SingleLine().Grow(1)
				ui.Text(c, fmt.Sprintf("%d 个", len(versions))).FontSize(10).
					TextColor(t.TextMuted.Alpha(0.6))
			})
			ui.Box(c).Height(1).Background(ui.Hex("#ffffff1f"))

			for i := range versions {
				m := versions[i]
				chosen := m.ID == def
				selected := m.ID == a.verSel
				row := ui.ButtonBase(c).Height(36).PaddingX(10).Radius(10).
					AlignItems(ui.Center).Gap(8)
				switch {
				case chosen:
					row.Background(ui.Hex("#e8763a1f"))
				case selected:
					row.Background(ui.Hex("#ffffff14"))
				case row.Hovered():
					row.Background(ui.Hex("#ffffff12"))
				}
				row.Children(func() {
					name := t.TextMuted
					if chosen {
						name = t.Accent
					}
					ui.Text(c, a.versionName(m)).FontSize(12).FontWeight(600).
						TextColor(name).SingleLine().Grow(1)
					if lang := langOf(m); lang != "" {
						ui.Text(c, lang).FontSize(9.5).TextColor(t.TextMuted.Alpha(0.7))
					}
					// The rom file itself, at the right end of the row: two dumps of
					// one title are told apart by this, and even with different titles
					// the file is what one opens in a zip manager to check.
					if rom := a.romTag(m); rom != "" {
						ui.Text(c, rom).FontSize(10).TextColor(t.TextMuted.Alpha(0.55)).
							SingleLine()
					}
					if chosen {
						ui.Text(c, "✓").FontSize(11).FontWeight(700).TextColor(t.Accent)
					}
				})
				if row.Clicked() {
					a.pickVersion(g, m.ID)
				}

				// This dump's own menu, on the right button: 取消合并, which takes
				// it out of the title's list and gives it a card of its own.
				vOpen := a.vmenuOpen(m.ID)
				vm := ui.ContextMenuBase(c, row, vOpen)
				if *vOpen {
					a.vmenuFor(m.ID)
					vm.Popup(func(panel ui.Element) { a.splitMenuPanel(c, t, panel, m) })
				} else if a.vmenuGame == m.ID {
					a.shutVMenu()
				}
			}
		})
	})
}

// pickVersion takes one click on a version row: that dump becomes the one that
// plays for the card, and the list closes. The card's cover follows the default,
// so the picture under the title changes with the pick.
func (a *App) pickVersion(g library.Game, id int) {
	a.setVersionDefault(g, id)
	a.verSel = id
	open := a.versionOpen(g.ID)
	*open = false
}

// splitMenuPanel is the menu of one version row: 取消合并, with the dump it is
// about and what the button does, and — below it — that dump's ips patches.
func (a *App) splitMenuPanel(c *ui.Context, t *ui.Theme, panel ui.Element, m library.Game) {
	panel.Width(268).Padding(7).Radius(14).
		Background(ui.Hex("#1b1d26f7")).
		Border(1, ui.Hex("#ffffff2e")).
		Shadow(0, 22, 48, 0, ui.Hex("#000000e6"))
	panel.Children(func() {
		ui.Column(c).Gap(2).Children(func() {
			ui.Row(c).Height(30).PaddingX(8).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, a.versionName(m)).FontSize(12).FontWeight(600).
					TextColor(t.Text).SingleLine().Grow(1)
				ui.Text(c, romNameOf(m)).FontSize(10).TextColor(t.TextMuted.Alpha(0.55)).
					SingleLine()
			})
			ui.Box(c).Height(1).Background(ui.Hex("#ffffff1f"))
			row := ui.ButtonBase(c).Height(34).PaddingX(9).Radius(9).
				AlignItems(ui.Center).Gap(8)
			if row.Hovered() {
				row.Background(ui.Hex("#ffffff10"))
			}
			row.Children(func() {
				ui.Text(c, "⇱").FontSize(12).TextColor(t.TextMuted.Alpha(0.85))
				ui.Text(c, "取消合并").FontSize(12).FontWeight(500).
					TextColor(t.Text.Alpha(0.9)).SingleLine().Grow(1)
				ui.Text(c, "只取消这一个").FontSize(10).TextColor(t.TextMuted.Alpha(0.6))
			})
			if row.Clicked() {
				a.splitDump(m.ID)
			}
			a.ipsSection(c, t, m)
		})
	})
}

// langOf is the language a dump is in, from the region the catalogue knows or
// from the title's own note.
func langOf(g library.Game) string {
	switch strings.ToLower(strings.TrimSpace(g.Region)) {
	case "japan", "jp", "j":
		return "日版"
	case "usa", "us", "u":
		return "美版"
	case "europe", "eur", "e":
		return "欧版"
	case "world", "w":
		return "世界版"
	case "asia":
		return "亚洲版"
	case "china", "cn", "c":
		return "中文"
	}
	t := g.Title
	switch {
	case strings.Contains(t, "中文") || strings.Contains(t, "汉化"):
		return "中文"
	case strings.Contains(t, "日版"):
		return "日版"
	case strings.Contains(t, "美版"):
		return "美版"
	case strings.Contains(t, "欧版"):
		return "欧版"
	case strings.Contains(t, "世界版"):
		return "世界版"
	}
	return ""
}
