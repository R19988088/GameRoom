package app

import (
	"fmt"
	"path/filepath"

	"github.com/R19988088/GameRoom/internal/library"
)

// ImportDOS is the no-window path: `GameRoom.exe -import-dos <旧目录>` rebuilds
// the DOS&PC98 shelf from the games already in rom/dos and rom/pc98 and the
// old collection's list.csv, leaving every other shelf untouched.
//
// It is a separate run rather than part of the first start because it rewrites
// two platforms and nothing else: the arcade, the consoles and their covers
// are already in the library, and re-reading the old frontend's data.dll would
// throw all of that away.
func ImportDOS(src string) error {
	root := findRoot()
	db, err := library.Open(filepath.Join(root, "数据", "launcher.db"))
	if err != nil {
		return fmt.Errorf("打不开数据库：%w", err)
	}
	defer db.Close()

	// Before the shelf is written: give every zip the start command its own
	// dosbox.conf already describes, so a double click lands in the game rather
	// than in DOSBox Pure's start menu. Safe to repeat — prepared zips are left
	// alone.
	rep, err := library.PrepareDOSLaunch(filepath.Join(root, "rom", library.DOSKey))
	if err != nil {
		return err
	}
	fmt.Printf("DOS 直启：新写入 %d，已有 %d，留给菜单 %d\n", rep.Prepared, rep.Already, len(rep.Skipped))
	for _, s := range rep.Skipped {
		fmt.Printf("  %s：%s\n", s.Zip, s.Reason)
	}

	n, err := library.ImportDOS(db, src, root)
	if err != nil {
		return err
	}
	fmt.Printf("DOS&PC98：写入 %d 个游戏\n", n)
	return nil
}
