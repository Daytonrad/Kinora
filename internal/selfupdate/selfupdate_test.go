package selfupdate

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func makeZip(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, body := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		w.Write([]byte(body))
	}
	zw.Close()
	return buf.Bytes()
}

func serve(t *testing.T, zipBody []byte, sum string) Release {
	t.Helper()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/pkg.zip":
			w.Write(zipBody)
		case "/pkg.zip.sha256":
			w.Write([]byte(sum + "  " + Asset + "\r\n"))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(ts.Close)
	return Release{Tag: "v1.2.0", ZipURL: ts.URL + "/pkg.zip", SumURL: ts.URL + "/pkg.zip.sha256"}
}

func hexSum(b []byte) string { s := sha256.Sum256(b); return hex.EncodeToString(s[:]) }

func noVersionCheck(t *testing.T) {
	old := CheckVersion
	CheckVersion = func(context.Context, string, string) error { return nil }
	t.Cleanup(func() { CheckVersion = old })
}

func read(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestPrepareChecksTheHash(t *testing.T) {
	noVersionCheck(t)
	body := makeZip(t, map[string]string{"kinora.exe": "new"})
	install := t.TempDir()
	if err := Prepare(context.Background(), http.DefaultClient, install, serve(t, body, strings.Repeat("0", 64))); err == nil || !strings.Contains(err.Error(), "SHA-256") {
		t.Fatalf("a wrong checksum must stop the update: %v", err)
	}
	if Staged(install) {
		t.Fatal("staged despite the wrong checksum")
	}
	if err := Prepare(context.Background(), http.DefaultClient, install, serve(t, body, strings.ToUpper(hexSum(body)))); err != nil {
		t.Fatal(err)
	}
	if !Staged(install) || StagedTag() != "v1.2.0" {
		t.Fatal("not staged")
	}
}

func TestPrepareRefusesEscapingPaths(t *testing.T) {
	noVersionCheck(t)
	for _, name := range []string{"../evil.exe", "/abs/evil.exe", `..\evil.exe`} {
		body := makeZip(t, map[string]string{"kinora.exe": "new", name: "x"})
		install := t.TempDir()
		if err := Prepare(context.Background(), http.DefaultClient, install, serve(t, body, hexSum(body))); err == nil {
			t.Errorf("%s: accepted", name)
		}
		if _, err := os.Stat(filepath.Join(filepath.Dir(install), "evil.exe")); err == nil {
			t.Fatalf("%s: written outside the install folder", name)
		}
	}
	body := makeZip(t, map[string]string{"README.md": "no exe"})
	if err := Prepare(context.Background(), http.DefaultClient, t.TempDir(), serve(t, body, hexSum(body))); err == nil {
		t.Error("a package without kinora.exe was accepted")
	}
}

// setup: an install with the old program, a database and a staged package that replaces two files and adds one.
func setup(t *testing.T) (install, data string) {
	t.Helper()
	install, data = t.TempDir(), t.TempDir()
	os.MkdirAll(filepath.Join(install, "ffmpeg"), 0o755)
	os.WriteFile(filepath.Join(install, "kinora.exe"), []byte("old"), 0o755)
	os.WriteFile(filepath.Join(install, "ffmpeg", "ffmpeg.exe"), []byte("old ffmpeg"), 0o755)
	os.WriteFile(filepath.Join(install, "keep.txt"), []byte("untouched"), 0o644)
	os.WriteFile(filepath.Join(data, "kinora.db"), []byte("db v1"), 0o644)
	stage := stageDir(install)
	os.MkdirAll(filepath.Join(stage, "ffmpeg"), 0o755)
	os.WriteFile(filepath.Join(stage, "kinora.exe"), []byte("new"), 0o755)
	os.WriteFile(filepath.Join(stage, "ffmpeg", "ffmpeg.exe"), []byte("new ffmpeg"), 0o755)
	os.WriteFile(filepath.Join(stage, "added.txt"), []byte("added"), 0o644)
	return install, data
}

func TestSwapConfirm(t *testing.T) {
	install, data := setup(t)
	if err := Swap(install, data, "v1.1.0", "v1.2.0"); err != nil {
		t.Fatal(err)
	}
	if read(t, filepath.Join(install, "kinora.exe")) != "new" || read(t, filepath.Join(install, "ffmpeg", "ffmpeg.exe")) != "new ffmpeg" ||
		read(t, filepath.Join(install, "keep.txt")) != "untouched" || read(t, filepath.Join(install, "added.txt")) != "added" {
		t.Fatal("files not swapped")
	}
	if Staged(install) || !Pending(install) {
		t.Fatal("stage left behind or no marker")
	}
	exit, probation := Guard(install, data, "kinora v1.2.0")
	if exit || !probation {
		t.Fatalf("first start: exit=%v probation=%v", exit, probation)
	}
	Confirm(install)
	if Pending(install) {
		t.Fatal("marker kept after confirm")
	}
	if log := read(t, filepath.Join(install, "update.log")); !strings.Contains(log, "  Sikeres frissítés: kinora v1.2.0") {
		t.Fatalf("update.log: %q", log)
	}
	if exit, probation := Guard(install, data, "kinora v1.2.0"); exit || probation {
		t.Fatal("probation after confirm")
	}
}

func TestGuardRollsBackAfterFailedStarts(t *testing.T) {
	install, data := setup(t)
	if err := Swap(install, data, "v1.1.0", "v1.2.0"); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(data, "kinora.db"), []byte("db v2, migrated"), 0o644)
	os.WriteFile(filepath.Join(data, "kinora.db-wal"), []byte("wal"), 0o644)
	for i := 1; i < maxStarts; i++ {
		if exit, probation := Guard(install, data, "v1.2.0"); exit || !probation {
			t.Fatalf("start %d: exit=%v probation=%v", i, exit, probation)
		}
	}
	exit, _ := Guard(install, data, "v1.2.0")
	if !exit {
		t.Fatal("no rollback on the last start")
	}
	if read(t, filepath.Join(install, "kinora.exe")) != "old" || read(t, filepath.Join(install, "ffmpeg", "ffmpeg.exe")) != "old ffmpeg" ||
		read(t, filepath.Join(install, "keep.txt")) != "untouched" {
		t.Fatal("program not restored")
	}
	if _, err := os.Stat(filepath.Join(install, "added.txt")); err == nil {
		t.Fatal("file added by the update kept")
	}
	if read(t, filepath.Join(data, "kinora.db")) != "db v1" {
		t.Fatal("database not restored")
	}
	if _, err := os.Stat(filepath.Join(data, "kinora.db-wal")); err == nil {
		t.Fatal("wal of the new version kept")
	}
	if Pending(install) {
		t.Fatal("marker kept after rollback")
	}
	log := read(t, filepath.Join(install, "update.log"))
	if !strings.Contains(log, "HIBA: ") || !strings.Contains(log, "Visszaállítva: v1.1.0") {
		t.Fatalf("update.log: %q", log)
	}
}

func TestGuardRollsBackOldProbation(t *testing.T) {
	install, data := setup(t)
	if err := Swap(install, data, "v1.1.0", "v1.2.0"); err != nil {
		t.Fatal(err)
	}
	m, _ := readMarker(install)
	m.Started = time.Now().Add(-rollbackAge - time.Minute)
	writeMarker(install, m)
	if exit, _ := Guard(install, data, "v1.2.0"); !exit {
		t.Fatal("an old probation must roll back")
	}
}

func TestGuardIgnoresOtherVersion(t *testing.T) {
	install, data := setup(t)
	if err := Swap(install, data, "v1.1.0", "v1.2.0"); err != nil {
		t.Fatal(err)
	}
	if exit, probation := Guard(install, data, "v1.3.0"); exit || probation || Pending(install) {
		t.Fatal("a different version must clear the marker without rollback")
	}
	if read(t, filepath.Join(install, "kinora.exe")) != "new" {
		t.Fatal("files touched")
	}
}
