package server

import (
	"strings"
	"testing"
)

func TestRewritePlaylistFMP4(t *testing.T) {
	in := "#EXTM3U\n#EXT-X-MAP:URI=\"init.mp4\"\n#EXTINF:4.0,\nseg00000.m4s\n"
	out := string(rewritePlaylist([]byte(in), "tok"))
	if !strings.Contains(out, `URI="init.mp4?st=tok"`) || !strings.Contains(out, "seg00000.m4s?st=tok") {
		t.Fatalf("rewrite: %s", out)
	}
	for _, f := range []string{"index.m3u8", "init.mp4", "seg00000.ts", "seg00012.m4s"} {
		if !reHLSFile.MatchString(f) {
			t.Fatalf("%s rejected", f)
		}
	}
	if reHLSFile.MatchString("seg00000.mp4") || reHLSFile.MatchString("x.m4s") {
		t.Fatal("bad names accepted")
	}
}
