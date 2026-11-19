package covers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// pngStub is a byte string that passes the PNG check: signature plus padding.
func pngStub() []byte {
	b := []byte("\x89PNG\r\n\x1a\n")
	for len(b) < 2000 {
		b = append(b, 'x')
	}
	return b
}

// TestCoverFallsBackToTheInGameShot: a romset with no title shot still gets
// the in-game one, and a romset ADB does not hold is ErrNoCover.
func TestCoverFallsBackToTheInGameShot(t *testing.T) {
	body := pngStub()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/titles/oplayer.png"):
			http.NotFound(w, r)
		case strings.HasSuffix(r.URL.Path, "/ingames/oplayer.png"):
			w.Header().Set("Content-Type", "image/png")
			w.Write(body)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	adb := &ADB{Client: srv.Client(), Base: srv.URL, Release: "209", Pause: 0}

	data, err := adb.Cover("oplayer")
	if err != nil {
		t.Fatalf("Cover: %v", err)
	}
	if len(data) != len(body) {
		t.Errorf("got %d bytes, want %d", len(data), len(body))
	}
	if _, err := adb.Cover("no-such-rom"); err != ErrNoCover {
		t.Errorf("missing romset: err = %v, want ErrNoCover", err)
	}
}

// TestCoverIgnoresAnErrorPage: a 200 that is not a picture is not a cover.
func TestCoverIgnoresAnErrorPage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte("<html>rate limited</html>"))
	}))
	defer srv.Close()

	adb := &ADB{Client: srv.Client(), Base: srv.URL, Release: "209", Pause: 0}
	if _, err := adb.Cover("kof98"); err != ErrNoCover {
		t.Errorf("err = %v, want ErrNoCover", err)
	}
}

// TestInfoReadsThePage: manufacturer, year and genre come off the detail page.
func TestInfoReadsThePage(t *testing.T) {
	page := `<div class="table_row new_row">
	  <div class="table_caption">Manufacturer: </div>
	  <div class="table_value"> <span class="dettaglio">SNK</span> <div>…</div></div>
	</div>
	<div class="table_row new_row">
	  <div class="table_caption">Year: </div>
	  <div class="table_value"> <span class="dettaglio">1998</span> <div>…</div></div>
	</div>
	<div class="table_row new_row">
	  <div class="table_caption">Genre: </div>
	  <div class="table_value"> <span class="dettaglio">Fighter</span></div>
	</div>`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(page))
	}))
	defer srv.Close()

	adb := &ADB{Client: srv.Client(), Base: srv.URL, Pause: time.Millisecond}
	maker, year, genre, err := adb.Info("kof98")
	if err != nil {
		t.Fatalf("Info: %v", err)
	}
	if maker != "SNK" || year != "1998" || genre != "Fighter" {
		t.Errorf("got %q %q %q", maker, year, genre)
	}
}

// TestInfoOfAnEmptyPage: a page without the table is no information.
func TestInfoOfAnEmptyPage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("<html><body>nothing here</body></html>"))
	}))
	defer srv.Close()

	adb := &ADB{Client: srv.Client(), Base: srv.URL, Pause: 0}
	if _, _, _, err := adb.Info("zzz"); err != ErrNoCover {
		t.Errorf("err = %v, want ErrNoCover", err)
	}
}
