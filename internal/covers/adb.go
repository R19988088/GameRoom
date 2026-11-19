// Package covers fetches the cover art a library of roms does not ship with.
//
// Arcade Italia (adb.arcadeitalia.net) publishes MAME's media tree at a plain
// path per romset:
//
//	https://adb.arcadeitalia.net/media/mame.current/titles/oplayer.png?release=209
//
// There is no key and no referer check: a romset it holds comes back as an
// image/png, one it does not as a 404. The title shot is the cover; for romsets
// that only have an in-game shot, that is the fallback.
package covers

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// adbMedia is the media tree: <base>/<kind>/<romset>.png?release=<n>.
const adbMedia = "https://adb.arcadeitalia.net/media/mame.current"

// adbDetail is the page per romset, with the table the launcher reads the
// manufacturer, year and genre from.
const adbDetail = "https://adb.arcadeitalia.net/dettaglio_mame.php?game_name="

// ErrNoCover says the romset is not one ADB holds a picture of.
var ErrNoCover = errors.New("ADB 没有这张封面")

// ADB is the client for that tree. One per run: it paces itself, because a
// library is hundreds of requests at a time.
type ADB struct {
	Client  *http.Client
	Release string        // the ?release= MAME version the media belongs to
	Pause   time.Duration // wait before every request
	Base    string        // "" for the real ADB; a test server otherwise
}

// media is the tree the pictures come from.
func (a *ADB) media() string {
	if a.Base != "" {
		return a.Base
	}
	return adbMedia
}

// detail is the page the metadata comes from.
func (a *ADB) detail(rom string) string {
	if a.Base != "" {
		return a.Base + "/dettaglio_mame.php?game_name=" + url.QueryEscape(rom)
	}
	return adbDetail + url.QueryEscape(rom)
}

// NewADB returns a client that keeps its requests a fifth of a second apart.
func NewADB() *ADB {
	return &ADB{
		Client:  &http.Client{Timeout: 25 * time.Second},
		Release: "209",
		Pause:   400 * time.Millisecond,
	}
}

// Cover downloads the art of a romset ("oplayer", "kof98"): the title shot,
// or the in-game shot when the title is missing. ErrNoCover when ADB has
// neither.
func (a *ADB) Cover(rom string) ([]byte, error) {
	rom = strings.ToLower(strings.TrimSpace(rom))
	if rom == "" {
		return nil, ErrNoCover
	}
	for _, kind := range []string{"titles", "ingames"} {
		data, err := a.get(kind, rom)
		if err == nil {
			return data, nil
		}
		if !errors.Is(err, ErrNoCover) {
			return nil, err
		}
	}
	return nil, ErrNoCover
}

// Info is what a romset's page says about the game: who built it, when, and
// what kind of game it is. Empty strings for what the page does not fill.
func (a *ADB) Info(rom string) (maker, year, genre string, err error) {
	rom = strings.ToLower(strings.TrimSpace(rom))
	if rom == "" {
		return "", "", "", ErrNoCover
	}
	if a.Pause > 0 {
		time.Sleep(a.Pause)
	}
	req, err := http.NewRequest(http.MethodGet, a.detail(rom), nil)
	if err != nil {
		return "", "", "", err
	}
	req.Header.Set("User-Agent", "GameRoom/1.0 (cover fetch)")
	resp, err := a.Client.Do(req)
	if err != nil {
		return "", "", "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", "", "", ErrNoCover
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return "", "", "", err
	}
	page := string(body)
	maker = adbValue(page, "Manufacturer")
	year = adbValue(page, "Year")
	genre = adbValue(page, "Genre")
	if maker == "" && year == "" {
		return "", "", "", ErrNoCover
	}
	return maker, year, genre, nil
}

// adbValue reads the value the page's table puts after a caption:
//
//	<div class="table_caption">Year: </div> <div class="table_value">
//	  <span class="dettaglio">1998</span> …
func adbValue(page, caption string) string {
	i := strings.Index(page, `<div class="table_caption">`+caption)
	if i < 0 {
		return ""
	}
	j := strings.Index(page[i:], `class="dettaglio">`)
	if j < 0 {
		return ""
	}
	rest := page[i+j+len(`class="dettaglio">`):]
	if k := strings.Index(rest, "<"); k >= 0 {
		rest = rest[:k]
	}
	return strings.TrimSpace(rest)
}

// get asks for one picture under one kind of the media tree.
func (a *ADB) get(kind, rom string) ([]byte, error) {
	if a.Pause > 0 {
		time.Sleep(a.Pause)
	}
	u := fmt.Sprintf("%s/%s/%s.png?release=%s", a.media(), kind, url.PathEscape(rom), url.QueryEscape(a.Release))
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "GameRoom/1.0 (cover fetch)")
	// ADB throttles a burst: a handshake that fails once usually works a
	// few seconds later, so a couple of retries keep a bulk run going.
	var resp *http.Response
	for try := 0; try < 3; try++ {
		resp, err = a.Client.Do(req)
		if err == nil {
			break
		}
		time.Sleep(time.Duration(2<<try) * time.Second)
	}
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	switch {
	case resp.StatusCode == http.StatusNotFound:
		return nil, ErrNoCover
	case resp.StatusCode != http.StatusOK:
		return nil, fmt.Errorf("ADB %s %s：%s", kind, rom, resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	// A picture, not an error page dressed as one.
	if len(data) < 1024 || !bytes.HasPrefix(data, []byte("\x89PNG\r\n\x1a\n")) {
		return nil, ErrNoCover
	}
	return data, nil
}
