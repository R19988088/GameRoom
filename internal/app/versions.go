package app

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

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
	members := a.verMembers[a.verFam[g.ID]]
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
		return g
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
	if err := library.SetSetting(a.db, versionPrefix+key, strconv.Itoa(id)); err != nil {
		a.status = "默认版本没存上：" + err.Error()
		return
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

// versionLabel is what a version is called under the base title: the dump's own
// tail ("霸王丸地狱变 sp"), or the whole title when the two titles do not
// share a base.
func (a *App) versionLabel(base, g library.Game) string {
	label := ""
	if g.ID == base.ID {
		label = "主 ROM"
	} else if strings.HasPrefix(g.Title, base.Title) {
		label = strings.TrimSpace(g.Title[len(base.Title):])
	}
	if label == "" {
		label = g.Title
	}
	// Two dumps can carry the same title ("麻将 大中华圈" twice); the rom file
	// is what tells them apart, so it is shown rather than a duplicate row.
	same := 0
	for _, m := range a.verMembers[a.verFam[g.ID]] {
		if m.Title == g.Title {
			same++
		}
	}
	if same > 1 {
		label += " · " + romStem(g.RomPath)
	}
	return label
}

// romStem is a rom's file name without its extension, as "samsho2".
func romStem(path string) string {
	base := path
	if i := strings.LastIndexAny(base, `/\`); i >= 0 {
		base = base[i+1:]
	}
	if i := strings.LastIndex(base, "."); i > 0 {
		base = base[:i]
	}
	return base
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
// a tick on the one that plays. A row takes the choice and a second click on it
// soon after makes it the default — the same two-step a card uses to play.
func (a *App) versionPanel(c *ui.Context, t *ui.Theme, panel ui.Element, g library.Game, versions []library.Game) {
	base := a.versionBase(g)
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
			ui.Row(c).Height(20).PaddingX(10).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "双击设为默认").FontSize(10).TextColor(t.TextMuted.Alpha(0.55))
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
					ui.Text(c, a.versionLabel(base, m)).FontSize(12).FontWeight(600).
						TextColor(name).SingleLine().Grow(1)
					if lang := langOf(m); lang != "" {
						ui.Text(c, lang).FontSize(9.5).TextColor(t.TextMuted.Alpha(0.7))
					}
					if chosen {
						ui.Text(c, "✓").FontSize(11).FontWeight(700).TextColor(t.Accent)
					}
				})
				if row.Clicked() {
					now := time.Now()
					if a.verClick == m.ID && now.Sub(a.verClickAt) < 650*time.Millisecond {
						a.verClick, a.verClickAt = 0, now
						a.setVersionDefault(g, m.ID)
						a.verSel = m.ID
						open := a.versionOpen(g.ID)
						*open = false
					} else {
						a.verClick, a.verClickAt = m.ID, now
						a.verSel = m.ID
					}
				}
			}
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
