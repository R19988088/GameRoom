package library

import (
	"archive/zip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// makeZip writes a small archive shaped like the collection's: an optional
// dosbox.conf at the root and a game folder holding a play.BAT.
func makeZip(t *testing.T, path, conf string, files map[string]string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	zw := zip.NewWriter(f)
	add := func(name, body string) {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if conf != "" {
		add("dosbox.conf", conf)
	}
	for name, body := range files {
		add(name, body)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
}

func readZipEntry(t *testing.T, path, name string) string {
	t.Helper()
	zr, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	for _, f := range zr.File {
		if zipPath(f.Name) != strings.ToLower(name) {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		defer rc.Close()
		buf := make([]byte, 0, 4096)
		chunk := make([]byte, 512)
		for {
			n, err := rc.Read(chunk)
			buf = append(buf, chunk[:n]...)
			if err != nil {
				break
			}
		}
		return string(buf)
	}
	return ""
}

// The archive says how to start the game; the batch we add repeats it without
// the old frontend's mount and exit, which is what Pure needs.
func TestLaunchBodyFollowsTheArchiveAutoexec(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "if.zip")
	makeZip(t, path, "[autoexec]\nmount C .\nC:\ncd IF2\ncall PLAY.BAT\nexit\n",
		map[string]string{"IF2/PLAY.BAT": "game"})

	res, err := prepareOne(path)
	if err != nil || res != "written" {
		t.Fatalf("prepareOne = %q, %v; want written", res, err)
	}
	body := readZipEntry(t, path, launchBat)
	for _, want := range []string{"cd IF2", "call PLAY.BAT"} {
		if !strings.Contains(body, want) {
			t.Fatalf("batch %q lacks %q", body, want)
		}
	}
	for _, unwanted := range []string{"mount C", "exit"} {
		if strings.Contains(strings.ToLower(body), strings.ToLower(unwanted)) {
			t.Fatalf("batch %q still carries %q", body, unwanted)
		}
	}
}

// A second import must not touch a game that already starts itself.
func TestPrepareTwiceIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "if.zip")
	makeZip(t, path, "[autoexec]\nC:\ncd IF2\ncall PLAY.BAT\nexit\n",
		map[string]string{"IF2/PLAY.BAT": "game"})

	if _, err := prepareOne(path); err != nil {
		t.Fatal(err)
	}
	res, err := prepareOne(path)
	if err != nil || res != "already" {
		t.Fatalf("second prepareOne = %q, %v; want already", res, err)
	}
	zr, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	n := 0
	for _, f := range zr.File {
		if zipPath(f.Name) == "dosbox.bat" {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("%d copies of DOSBOX.BAT, want 1", n)
	}
}

// Two archives shipped without a dosbox.conf; their own play.BAT is the
// launcher, and if even that is missing the game has to stay on the menu.
func TestLaunchBodyFallsBackToPlayBat(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tenchi.zip")
	makeZip(t, path, "", map[string]string{
		"TENCHI/PLAY.BAT":   "game",
		"TENCHI/PLAYX.BAT":  "extra",
		"TENCHI/TENCHI.EXE": "exe",
	})
	res, err := prepareOne(path)
	if err != nil || res != "written" {
		t.Fatalf("prepareOne = %q, %v; want written", res, err)
	}
	body := readZipEntry(t, path, launchBat)
	if !strings.Contains(body, "cd TENCHI") || !strings.Contains(body, "call PLAY.BAT") {
		t.Fatalf("batch %q does not start play.BAT", body)
	}

	empty := filepath.Join(dir, "nothing.zip")
	makeZip(t, empty, "", map[string]string{"DISC/DOSV.BAT": "font"})
	res, err = prepareOne(empty)
	if err != nil {
		t.Fatal(err)
	}
	if res == "written" || res == "already" {
		t.Fatalf("prepareOne on a game with no launcher = %q, want a reason", res)
	}
}

// A batch that points at a folder the zip does not have would open on a dead
// command line, so it is refused at import time.
func TestLaunchBodyRefusesMissingFolder(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.zip")
	makeZip(t, path, "[autoexec]\nC:\ncd MISSING\nPLAY.BAT\nexit\n",
		map[string]string{"OTHER/PLAY.BAT": "game"})
	res, err := prepareOne(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res, "cd MISSING") {
		t.Fatalf("reason = %q, want it to name the missing folder", res)
	}
}
