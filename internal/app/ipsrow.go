package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/R19988088/GameRoom/internal/ips"
	"github.com/R19988088/GameRoom/internal/library"
)

// ipsDirs are the folders a dump's patches may live in, under 数据/ips: one
// named after the rom file — the way a MAME Plus collection names a set —
// then the platform's own subfolder, then the game's title, for a folder that
// was organised by hand.
func (a *App) ipsDirs(g library.Game) []string {
	base := filepath.Join(a.root, "数据", "ips")
	stem := romStem(g.RomPath)
	seen := map[string]bool{}
	var out []string
	add := func(p string) {
		if p == "" || seen[p] {
			return
		}
		st, err := os.Stat(p)
		if err != nil || !st.IsDir() {
			return
		}
		seen[p] = true
		out = append(out, p)
	}
	add(filepath.Join(base, stem))
	add(filepath.Join(base, g.PlatformKey, stem))
	if title := strings.TrimSpace(g.Title); title != "" && title != stem {
		add(filepath.Join(base, title))
	}
	return out
}

// ipsGroups are the patch groups this dump could play with: one per .dat in its
// ips folder, and one per loose .ips. A dump with no folder has none — the menu
// then shows no ips section at all.
func (a *App) ipsGroups(g library.Game) []ips.Group {
	seen := map[string]bool{}
	var out []ips.Group
	for _, dir := range a.ipsDirs(g) {
		for _, gr := range ips.Scan(dir) {
			if seen[gr.Name] {
				continue
			}
			seen[gr.Name] = true
			out = append(out, gr)
		}
	}
	return out
}

// ipsEnabled are the groups switched on for this dump that are still on disk:
// a group whose folder went away simply stops counting.
func (a *App) ipsEnabled(g library.Game) []ips.Group {
	var out []ips.Group
	for _, gr := range a.ipsGroups(g) {
		if g.IPSOn(gr.Name) {
			out = append(out, gr)
		}
	}
	return out
}

// toggleIPS switches one patch group on or off, in the row it was clicked on.
// The write is immediate: nothing in the body changes, and the switch has to
// show its new state on the next frame.
func (a *App) toggleIPS(g library.Game, group string) {
	on := !g.IPSOn(group)
	groups := append([]string(nil), g.IPSGroups()...)
	if on {
		groups = append(groups, group)
	} else {
		kept := make([]string, 0, len(groups))
		for _, n := range groups {
			if n != group {
				kept = append(kept, n)
			}
		}
		groups = kept
	}
	before := g.IPS
	if gg := a.lib.Game(g.ID); gg != nil {
		gg.IPS = strings.Join(groups, "|")
	}
	if a.db != nil {
		if err := library.SetIPS(a.db, g.ID, groups); err != nil {
			if gg := a.lib.Game(g.ID); gg != nil {
				gg.IPS = before
			}
			a.status = "ips 没记住：" + err.Error()
			return
		}
	}
	if on {
		a.status = fmt.Sprintf("%s：ips %s 已启用", g.Title, group)
	} else {
		a.status = fmt.Sprintf("%s：ips %s 已关闭", g.Title, group)
	}
}

// ipsSection is the menu's ips part, at its bottom: the label, then one row per
// patch group with a switch at its right. Nothing is drawn when the dump has no
// folder under 数据/ips — a menu only offers what that dump really has.
func (a *App) ipsSection(c *ui.Context, t *ui.Theme, g library.Game) {
	groups := a.ipsGroups(g)
	if len(groups) == 0 {
		return
	}
	ui.Box(c).Height(1).Background(ui.Hex("#ffffff1f"))
	ui.Row(c).Height(26).PaddingX(9).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Text(c, "ips").FontSize(12.5).FontWeight(700).TextColor(t.Accent)
		ui.Box(c).Height(1).Grow(1)
		ui.Text(c, fmt.Sprintf("%d 组", len(groups))).FontSize(10).
			TextColor(t.TextMuted.Alpha(0.55))
	})
	for _, gr := range groups {
		gr := gr
		row := ui.ButtonBase(c).Height(30).PaddingX(9).Radius(9).
			AlignItems(ui.Center).Gap(8)
		if row.Hovered() {
			row.Background(ui.Hex("#ffffff10"))
		}
		row.Children(func() {
			ui.Text(c, gr.Name).FontSize(11.5).FontWeight(500).
				TextColor(t.Text.Alpha(0.9)).SingleLine().Grow(1)
			// The same switch the filter panel uses, at its size: a 44×26 track.
			if switchTrack(c, g.IPSOn(gr.Name)) {
				a.toggleIPS(g, gr.Name)
			}
		})
		row.Label("ips " + gr.Name)
	}
}

// ipsTitle is the window's caption while a game plays: its title, and how many
// patch groups are switched on, so the title bar says what the picture is running
// with.
func ipsTitle(title string, groups int) string {
	if groups <= 0 {
		return title
	}
	return fmt.Sprintf("%s    ips启用 %d 组", title, groups)
}

// isFBNeoCore reports whether the core is one of the FBNeo builds. Those read a
// patched set of their own — <system>/fbneo/patched/<set>.zip — with the
// checksums ignored and only the names and sizes checked, which is the one way
// to run a rewritten set on them: a patched zip handed over as the rom itself is
// refused ("ROM … is missing", the checksum it wants is the old one).
// isFBNeoCore says whether a core is one of the FinalBurn builds: they share the
// patched-set mechanism, and they share the announced-but-not-read button table
// the pad mapping has to ignore.
func isFBNeoCore(name string) bool {
	n := strings.ToLower(strings.TrimSuffix(filepath.Base(name), ".dll"))
	n = strings.TrimSuffix(n, "_libretro")
	return strings.HasPrefix(n, "fbneo") || strings.HasPrefix(n, "fbalpha")
}

// patchedSetPath is where an FBNeo build looks for a set that differs from the
// library's: a file named after the set, inside the system folder it is handed.
func patchedSetPath(systemDir, rom string) string {
	return filepath.Join(systemDir, "fbneo", "patched", filepath.Base(rom))
}

// stagePatchedSet puts the patched copy where the core will find it, replacing
// whatever an earlier launch left there.
func stagePatchedSet(systemDir, rom, patched string) error {
	dst := patchedSetPath(systemDir, rom)
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	os.Remove(dst)
	if err := os.Link(patched, dst); err == nil {
		return nil
	}
	// Another volume, or a filesystem without hard links: the bytes are copied
	// once instead.
	data, err := os.ReadFile(patched)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o644)
}

// clearPatchedSet takes a patched set back out, so a game whose patches were
// switched off runs on the library's own rom again instead of yesterday's copy.
func clearPatchedSet(systemDir, rom string) {
	os.Remove(patchedSetPath(systemDir, rom))
}
