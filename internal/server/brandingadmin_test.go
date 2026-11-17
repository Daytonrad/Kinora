package server

import (
	"bytes"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestSniffAndSVGChecks(t *testing.T) {
	cases := map[string]string{"\x89PNG\r\n\x1a\nxxxx": "png", "\xff\xd8\xff\xe0": "jpg", "RIFF\x00\x00\x00\x00WEBPVP8 ": "webp",
		"  <svg xmlns='http://www.w3.org/2000/svg'/>": "svg", "<?xml version='1.0'?><svg/>": "svg", "GIF89a": "", "<html><svg/>": ""}
	for in, want := range cases {
		if got := sniffLogo([]byte(in)); got != want {
			t.Errorf("sniff %q = %q, want %q", in[:min(len(in), 12)], got, want)
		}
	}
	ok := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 10 10"><rect width="10" height="10" fill="#f0b44c"/><image href="data:image/png;base64,AAAA"/></svg>`
	if err := checkSVG([]byte(ok)); err != nil {
		t.Fatalf("clean svg rejected: %v", err)
	}
	for _, bad := range []string{
		`<svg><script>alert(1)</script></svg>`,
		`<svg onload="alert(1)"/>`,
		`<svg><a href="javascript:alert(1)"><rect/></a></svg>`,
		`<svg><image href="https://evil.example/x.png"/></svg>`,
		`<svg><use xlink:href="//evil.example/s.svg#a"/></svg>`,
		`<svg><foreignObject><iframe src="x"/></foreignObject></svg>`,
		`<!DOCTYPE svg [<!ENTITY x SYSTEM "file:///etc/passwd">]><svg>&x;</svg>`,
		`<svg><style>@import url(https://evil.example/a.css);</style></svg>`,
		`<svg><image href="data:text/html;base64,PHNjcmlwdD4="/></svg>`,
	} {
		if checkSVG([]byte(bad)) == nil {
			t.Errorf("unsafe svg accepted: %s", bad)
		}
	}
}

func TestInitialMediaAndKind(t *testing.T) {
	if n, p := initialMedia(" Filmek=/mnt/tar/Filmek "); n != "Filmek" || p != "/mnt/tar/Filmek" {
		t.Fatalf("named: %q %q", n, p)
	}
	if n, p := initialMedia("/srv/Sorozatok/"); n != "Sorozatok" || p != "/srv/Sorozatok" {
		t.Fatalf("plain: %q %q", n, p)
	}
	if _, p := initialMedia("relativ/ut"); p != "" {
		t.Fatal("relative paths must be ignored")
	}
	for in, want := range map[string]string{"Sorozatok /x": "shows", "TV Shows": "shows", "Filmek": "movies", "Movies 4K": "movies", "Családi videók": "mixed"} {
		if got := guessKind(in); got != want {
			t.Errorf("guessKind(%q) = %s, want %s", in, got, want)
		}
	}
}

func TestLogoUploadMakesIcons(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg missing")
	}
	c, _, srv := newTestServerFull(t)
	src := filepath.Join(t.TempDir(), "logo.png")
	if b, err := exec.Command("ffmpeg", "-hide_banner", "-loglevel", "error", "-f", "lavfi", "-i", "color=c=0xf0b44c:s=1200x600", "-frames:v", "1", src).CombinedOutput(); err != nil {
		t.Fatalf("ffmpeg: %v %s", err, b)
	}
	png, _ := os.ReadFile(src)
	up := func(variant string, body []byte) (int, map[string]any) {
		req, _ := http.NewRequest("POST", c.base+"/api/admin/branding/logo?variant="+variant, bytes.NewReader(body))
		req.Header.Set("Content-Type", "image/png")
		resp, err := c.hc.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var out map[string]any
		jsonDecode(resp.Body, &out)
		return resp.StatusCode, out
	}
	code, br := up("dark", png)
	if code != 200 || br["logo_dark"] != "/api/branding/logo-dark.png" || br["favicon"] != "/api/branding/favicon.png" || br["icon_512"] == "" {
		t.Fatalf("upload %d %v", code, br)
	}
	for _, f := range []string{"logo-dark.png", "favicon.png", "icon-192.png", "icon-512.png"} {
		if !fileExists(filepath.Join(srv.brandingDir(), f)) {
			t.Fatalf("%s not generated", f)
		}
	}
	// served as an image with a strict CSP, and public (apps show it before login)
	resp, err := http.Get(c.base + "/api/branding/logo-dark.png")
	if err != nil || resp.StatusCode != 200 || resp.Header.Get("Content-Type") != "image/png" ||
		resp.Header.Get("X-Content-Type-Options") != "nosniff" || resp.Header.Get("Content-Security-Policy") == "" {
		t.Fatalf("serve logo: %v %+v", err, resp)
	}
	resp.Body.Close()
	if code, _ := up("dark", []byte("<svg onload='alert(1)'/>")); code != 400 {
		t.Fatalf("unsafe svg: %d", code)
	}
	if code, _ := up("dark", []byte("GIF89a not an allowed format")); code != 400 {
		t.Fatalf("gif: %d", code)
	}
	if code, _ := up("dark", append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, maxLogoBytes+10)...)); code != 400 {
		t.Fatalf("too big: %d", code)
	}
	if code, _ := up("sideways", png); code != 400 {
		t.Fatalf("bad variant: %d", code)
	}
	// non-admins cannot change branding
	anon := &client{t: t, base: c.base, hc: &http.Client{}}
	if code := anon.do("POST", "/api/admin/branding/logo?variant=dark", nil, nil); code != 401 {
		t.Fatalf("anonymous upload %d", code)
	}
	if code := c.do("DELETE", "/api/admin/branding/logo?variant=dark", nil, nil); code != 200 || fileExists(filepath.Join(srv.brandingDir(), "favicon.png")) {
		t.Fatalf("delete logo %d", code)
	}
}

func TestStartupBrandingFromEnv(t *testing.T) {
	_, st, srv := newTestServerFull(t)
	media := t.TempDir()
	os.MkdirAll(filepath.Join(media, "Sorozatok"), 0o755)
	t.Setenv("KINORA_INITIAL_SERVER_NAME", "Máté Mozi")
	t.Setenv("KINORA_INITIAL_MEDIA", "Filmek="+media+";"+filepath.Join(media, "Sorozatok")+";/nincs/ilyen")
	st.DB.Exec(`DELETE FROM libraries`)
	srv.ApplyStartupBranding()
	if st.Setting("server_name") != "Máté Mozi" {
		t.Fatalf("server name %q", st.Setting("server_name"))
	}
	libs, _ := st.Libraries()
	if len(libs) != 2 || libs[0].Kind == "" {
		t.Fatalf("initial libraries %+v", libs)
	}
	// applied once: a later change in the admin UI is not overwritten on restart
	st.SetSetting("server_name", "Új név")
	srv.ApplyStartupBranding()
	if st.Setting("server_name") != "Új név" {
		t.Fatal("initial settings re-applied")
	}
}
