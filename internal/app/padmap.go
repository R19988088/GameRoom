package app

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/R19988088/GameRoom/internal/core"
	"github.com/R19988088/GameRoom/internal/library"
)

// padLate reads the core's button table again once it has spoken. FBNeo hands its
// table over after the first frame, not during load, so the read at launch is empty
// on that core and the panel would keep its drawn order — the wrong one for a
// six-panel cabinet (its Y is Button 1, its B is Button 4). Called from the core's
// own frame hook, so reading the table stays on the thread that writes it.
func (a *App) padLate(sess *core.Session) {
	if a.padIDsDone {
		return
	}
	descs := sess.InputDescs()
	if len(descs) == 0 {
		return
	}
	a.padIDsDone = true
	l := padPanelFor(padLayoutFor(a.padLive), descs)
	ids := padIDsFor(l, descs)
	a.win.Update(func() {
		if a.core != sess {
			return // another game, or the user, already took the window back
		}
		a.padNow, a.padIDs = l, ids
		a.padCheckStart(a.padGame, a.padCore, l, a.padProfile(a.padLive), descs, ids)
	})
}

// --- the sheet the launcher writes about the buttons ---

// padCheckPath is 数据/按键自检.txt: what the launcher believes the running core's
// buttons are. A player who says "the keys are wrong" hands this back instead of
// describing, in words, which key does what — the core's own names and what each
// key came out as are in it.
func (a *App) padCheckPath() string {
	if a.root == "" {
		return ""
	}
	return filepath.Join(a.root, "数据", "按键自检.txt")
}

// padCheckStart writes the table once per launch: the game, the core, the buttons
// the core announced, and what each panel button presses.
func (a *App) padCheckStart(g library.Game, coreFile string, l *padLayout, prof map[string]padBind, descs []core.InputDesc, ids map[string][]int) {
	p := a.padCheckPath()
	if p == "" {
		return
	}
	var b strings.Builder
	fmt.Fprintf(&b, "GameRoom 按键自检  %s\n", time.Now().Format("2006-01-02 15:04:05"))
	// The exe's own stamp, so a report always says which build made it.
	if exe, err := os.Executable(); err == nil {
		if st, err := os.Stat(exe); err == nil {
			fmt.Fprintf(&b, "build  %s\n", st.ModTime().Format("2006-01-02 15:04:05"))
		}
	}
	fmt.Fprintf(&b, "game   %s\nrom    %s\ncore   %s\n", g.Title, g.RomPath, filepath.Base(coreFile))
	if l == nil {
		b.WriteString("panel  （还没有面板）\n")
	} else {
		fmt.Fprintf(&b, "panel  %s  %d 键\n", l.ID, buttonsOf(l))
	}
	b.WriteString("\n核心公告（端口 0）:\n")
	if len(descs) == 0 {
		b.WriteString("  （这个核心什么都没说 → 面板照自己画的 id 按）\n")
	}
	for _, d := range descs {
		if d.Port != 0 || d.Device != 1 || d.Index != 0 {
			continue
		}
		fmt.Fprintf(&b, "  id=%-3d %s\n", d.ID, d.Name)
	}
	b.WriteString("\n面板 → 要按的 id:\n")
	if l != nil {
		for _, s := range l.Spots {
			bd := prof[s.ID]
			one, ok := ids[s.ID]
			target := "（面板自己的 id）"
			if ok {
				target = fmt.Sprint(one)
			}
			fmt.Fprintf(&b, "  %-6s %-6q  id=%-6s key=%-6q vk=%-3d pad=%d  → %s\n",
				s.ID, s.Label, s.ID, bd.Key, bd.VK, bd.Pad, target)
		}
	}
	b.WriteString("\n按键记录:\n")
	a.padLogHead, a.padLog = b.String(), ""
	if err := os.WriteFile(p, []byte(a.padLogHead), 0o644); err != nil {
		a.padLogHead = ""
	}
}

// padCheckKeys appends the keys held now, with what they came out as, but only
// when that changes: a session writes a line per press, not per frame.
func (a *App) padCheckKeys(l *padLayout, prof map[string]padBind, keys map[int]bool, pads map[int]map[int]bool, held map[int]bool) {
	p := a.padCheckPath()
	if p == "" || a.padLogHead == "" || l == nil {
		return
	}
	var downs []int
	for k := range keys {
		if keys[k] {
			downs = append(downs, k)
		}
	}
	sort.Ints(downs)
	var spotText []string
	for _, s := range l.Spots {
		b := prof[s.ID]
		if bindDown(b, keys, pads) {
			spotText = append(spotText, fmt.Sprintf("%s(%q pad%d/vk%d)", s.ID, s.Label, b.Pad, b.VK))
		}
	}
	var out []int
	for id := range held {
		if held[id] {
			out = append(out, id)
		}
	}
	sort.Ints(out)
	line := fmt.Sprintf("  按下键盘 vk=%v  命中 %v  → 发给核心 held %v", downs, spotText, out)
	if line == a.padLog {
		return
	}
	a.padLog = line
	f, err := os.OpenFile(p, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintln(f, line)
}


// --- what each button of a panel presses, per core ---

// retroNames are the words a core uses when it is talking about the retropad
// itself instead of the machine's own buttons: A, B, X, Y, L, R and the rest of
// the generic pad. A core whose buttons are only named like this is positional —
// MAME is — and its order is the one the panel is drawn with, so the panel keeps
// its own mapping for it.
var retroNames = map[string]bool{
	"a": true, "b": true, "x": true, "y": true,
	"l": true, "r": true, "l2": true, "r2": true, "l3": true, "r3": true,
	"select": true, "start": true, "coin": true, "mode": true,
	"up": true, "down": true, "left": true, "right": true,
	"d-pad up": true, "d-pad down": true, "d-pad left": true, "d-pad right": true,
}

// padIDsFor reads a panel against the names the running core gave its buttons,
// and answers what each spot should press. A core that names the machine's own
// buttons — "Button 1", "Weak Punch", "Button A", "Attack" — decides the order:
// FBNeo reads its Button 1 from B but Button 2 from A, and a Street Fighter
// cabinet puts the punches on Y/X/L, so the panel's "1" is not B on every core.
// A core that only names the retropad (MAME) leaves the panel's own ids alone; so
// does a core that announces nothing. Nothing at all is drawn differently — only
// what the buttons press changes.
func padIDsFor(l *padLayout, descs []core.InputDesc) map[string][]int {
	if l == nil || len(descs) == 0 {
		return nil
	}
	// FBNeo announces its table later than the others — after the first frame, not
	// during load — so a launch that read it too early used to fall back on the
	// drawn order and came out wrong (its 6-panel says Y=1, X=2, L=3, B=4, A=5,
	// R=6, while the panel draws B=1 … R=6). The launch calls this again once the
	// core has spoken (padLate); from then on its names decide, like a console
	// core's do.
	names := map[string]int{}
	byNumber := map[int]int{}
	machine := false
	for _, d := range descs {
		if d.Port != 0 || d.Device != 1 || d.Index != 0 {
			continue // port 1 is the second player, the analog and gun devices are not buttons
		}
		n := strings.ToLower(strings.TrimSpace(d.Name))
		if n == "" {
			continue
		}
		if _, seen := names[n]; seen {
			continue
		}
		names[n] = d.ID
		if !retroNames[n] {
			machine = true
		}
		// A core that numbers its own buttons — MAME's "B1: Jab Punch", a driver's
		// "P1 Button 3" — tells the panel where each machine button sits. MAME 2003
		// Plus is the reason this exists: it reads Button 1 from B, Button 2 from A
		// and Button 3 from Y, so the panel's "2" and "3" land on each other's
		// buttons unless the number in the name is what is followed.
		if num := buttonNumberOf(n); num > 0 {
			if _, seen := byNumber[num]; !seen {
				byNumber[num] = d.ID
			}
		}
	}
	if !machine {
		return nil
	}

	ids := map[string][]int{}
	named := 0
	numbered, numberedNamed := 0, 0
	for _, s := range l.Spots {
		if len(s.Drives) > 1 {
			continue // a macro presses whatever its buttons press; done below
		}
		if _, err := strconv.Atoi(strings.TrimSpace(s.Label)); err == nil {
			numbered++
		}
		if num, err := strconv.Atoi(strings.TrimSpace(s.Label)); err == nil {
			// The machine's own numbering comes first: it says a machine button by
			// number, and the number is what the panel's "1"…"6" mean.
			if id, ok := byNumber[num]; ok {
				ids[s.ID] = []int{id}
				named++
				numberedNamed++
				continue
			}
		}
		if id, ok := spotIDFor(names, s.Label); ok {
			ids[s.ID] = []int{id}
			named++
			if _, err := strconv.Atoi(strings.TrimSpace(s.Label)); err == nil {
				numberedNamed++
			}
			continue
		}
		if v, ok := padRetroIds[targetsOf(s)[0]]; ok && v >= 0 {
			ids[s.ID] = []int{v}
		}
	}
	if numbered > 0 && numberedNamed == 0 {
		// A panel of numbered buttons against a machine that numbers nothing —
		// dino's "Attack" and "Jump", say. The announce order is the only order
		// there is, and it is the machine's own: first button, second button.
		// Only the attack buttons count: the coin and start bars are named too.
		ordered := machineButtons(descs, names)
		var slots []padSpot
		for _, s := range l.Spots {
			if _, err := strconv.Atoi(strings.TrimSpace(s.Label)); err == nil && len(s.Drives) <= 1 {
				slots = append(slots, s)
			}
		}
		sort.Slice(slots, func(i, j int) bool {
			a, _ := strconv.Atoi(slots[i].Label)
			b, _ := strconv.Atoi(slots[j].Label)
			return a < b
		})
		for i := 0; i < len(slots) && i < len(ordered); i++ {
			ids[slots[i].ID] = []int{ordered[i]}
			named++
		}
	}
	if named == 0 {
		return nil // nothing of this panel is named: the drawn order stands
	}
	// A macro presses the buttons it names, at the ids those buttons turned out
	// to be: ABC is the three punches even where the punches moved to Y/X/L.
	for _, s := range l.Spots {
		if len(s.Drives) <= 1 {
			continue
		}
		var pressed []int
		for _, drv := range s.Drives {
			if one := ids[drv]; len(one) == 1 {
				pressed = append(pressed, one[0])
				continue
			}
			if v, ok := padRetroIds[drv]; ok && v >= 0 {
				pressed = append(pressed, v)
			}
		}
		if len(pressed) > 0 {
			ids[s.ID] = pressed
		}
	}
	return ids
}

// buttonNumRe finds the machine's own button number in a descriptor name:
// MAME 2003 Plus announces "B1: Jab Punch"…"B6: Roundhouse Kick", a driver's
// own input names read "P1 Button 3", MAME's older dumps "P1_BUTTON1". "Buttons
// 3x Punch" is three buttons at once, not button 3, and stays out.
var buttonNumRe = regexp.MustCompile(`(?i)(?:\bb([1-9])\b|\bbutton[ _]?([1-9])\b|\bfire[ _]?([1-9])\b)`)

// buttonNumberOf is the machine button a descriptor name is numbered as, 0 when
// it carries no number.
func buttonNumberOf(name string) int {
	m := buttonNumRe.FindStringSubmatch(name)
	if m == nil {
		return 0
	}
	for _, g := range m[1:] {
		if g != "" {
			n, _ := strconv.Atoi(g)
			return n
		}
	}
	return 0
}

// spotIDFor is the id a core gave the button a spot is labelled with: its number,
// or its name. A cabinet's "1" is the core's "Button 1", and on a Street Fighter
// layout that is the core's "Weak Punch".
func spotIDFor(names map[string]int, label string) (int, bool) {
	for _, cand := range spotNames(label) {
		if id, ok := names[cand]; ok {
			return id, true
		}
	}
	return 0, false
}

// spotNames are the names a core may give the button a spot is labelled with.
func spotNames(label string) []string {
	l := strings.ToLower(strings.TrimSpace(label))
	var cands []string
	switch l {
	case "1":
		cands = []string{"button 1", "fire 1", "weak punch"}
	case "2":
		cands = []string{"button 2", "fire 2", "medium punch"}
	case "3":
		cands = []string{"button 3", "fire 3", "strong punch"}
	case "4":
		cands = []string{"button 4", "fire 4", "weak kick"}
	case "5":
		cands = []string{"button 5", "fire 5", "medium kick"}
	case "6":
		cands = []string{"button 6", "fire 6", "strong kick"}
	case "a", "b", "c", "d":
		// A NeoGeo panel names its four buttons A to D instead of numbering them.
		cands = []string{"button " + l, "fire " + l}
	}
	return append(cands, l)
}

// machineButtons are the core's own buttons in the order it announced them —
// what a numbered panel falls back on when the machine numbers nothing.
func machineButtons(descs []core.InputDesc, names map[string]int) []int {
	out := make([]int, 0, 8)
	seen := map[int]bool{}
	for _, d := range descs {
		if d.Port != 0 || d.Device != 1 || d.Index != 0 {
			continue
		}
		n := strings.ToLower(strings.TrimSpace(d.Name))
		if n == "" || retroNames[n] || strings.Contains(n, "buttons") ||
			strings.Contains(n, "+") || strings.Contains(n, "turbo") ||
			strings.Contains(n, "stick") || strings.Contains(n, "gun") ||
			strings.Contains(n, "trackball") || strings.Contains(n, "mouse") ||
			strings.Contains(n, "coin") || strings.Contains(n, "disk") ||
			strings.Contains(n, "sensor") {
			continue
		}
		if id, ok := names[n]; !ok || id != d.ID {
			continue // the first use of the name is the one a panel goes by
		}
		if seen[d.ID] {
			continue
		}
		seen[d.ID] = true
		out = append(out, d.ID)
	}
	return out
}
