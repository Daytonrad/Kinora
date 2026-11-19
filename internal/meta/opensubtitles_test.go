package meta

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ScotyLucas/kinora/internal/store"
)

func TestOSHash(t *testing.T) {
	p := filepath.Join(t.TempDir(), "v.bin")
	data := make([]byte, 200000)
	for i := range data {
		data[i] = byte(i * 7)
	}
	if err := os.WriteFile(p, data, 0o644); err != nil {
		t.Fatal(err)
	}
	want := uint64(len(data))
	for _, off := range []int{0, len(data) - 65536} {
		for i := 0; i < 65536; i += 8 {
			want += binary.LittleEndian.Uint64(data[off+i:])
		}
	}
	got, err := OSHash(p)
	if err != nil || got != fmt.Sprintf("%016x", want) {
		t.Fatalf("OSHash = %q %v", got, err)
	}
	small := filepath.Join(t.TempDir(), "s.bin")
	os.WriteFile(small, data[:1000], 0o644)
	if _, err := OSHash(small); err == nil {
		t.Fatal("too small file should fail")
	}
}

func TestOpenSubtitlesSearchDownload(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/file/") && r.Header.Get("Api-Key") != "k" || !strings.HasPrefix(r.Header.Get("User-Agent"), "Kinora") {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/subtitles":
			if got := r.URL.Query().Get("imdb_id"); got != "133093" {
				t.Errorf("imdb_id = %q", got)
			}
			if got := r.URL.Query().Get("languages"); got != "en,hu" {
				t.Errorf("languages = %q", got)
			}
			fmt.Fprint(w, `{"data":[
			 {"attributes":{"language":"en","download_count":900,"release":"Matrix.1999.1080p","files":[{"file_id":1,"file_name":"a.srt"}]}},
			 {"attributes":{"language":"hu","download_count":10,"release":"Matrix HU","files":[{"file_id":2,"file_name":"b.srt"}]}},
			 {"attributes":{"language":"hu","download_count":5,"moviehash_match":true,"release":"Matrix HU exact","files":[{"file_id":3,"file_name":"c.srt"}]}},
			 {"attributes":{"language":"hu","download_count":50,"release":"two cds","files":[{"file_id":4},{"file_id":5}]}}]}`)
		case "/download":
			var in map[string]any
			json.NewDecoder(r.Body).Decode(&in)
			fmt.Fprintf(w, `{"link":%q,"file_name":"c.srt","remaining":4}`, srv.URL+"/file/3")
		case "/file/3":
			fmt.Fprint(w, "1\n00:00:01,000 --> 00:00:02,000\nSzia\n")
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	o := NewOpenSubtitles(st, "Kinora vtest")
	o.BaseURL = srv.URL
	if o.Enabled() {
		t.Fatal("enabled without a key")
	}
	if _, err := o.Search(context.Background(), SubtitleQuery{ImdbID: "tt0133093"}); err != ErrSubsNotConfigured {
		t.Fatalf("want ErrSubsNotConfigured, got %v", err)
	}
	st.SetSetting("opensubtitles_key", "k")
	res, err := o.Search(context.Background(), SubtitleQuery{ImdbID: "tt0133093", Langs: []string{"hun", "eng"}})
	if err != nil {
		t.Fatal(err)
	}
	ids := []int64{}
	for _, r := range res {
		ids = append(ids, r.FileID)
	}
	// Hungarian first, the exact match before the more popular one; the 2-CD one is skipped
	if fmt.Sprint(ids) != "[3 2 1]" || res[0].Lang != "hun" || !res[0].HashMatch {
		t.Fatalf("order = %v, first = %+v", ids, res[0])
	}
	d, err := o.Download(context.Background(), 3)
	if err != nil || d.Remaining != 4 || !strings.Contains(string(d.Data), "Szia") {
		t.Fatalf("download: %+v %v", d, err)
	}
}
