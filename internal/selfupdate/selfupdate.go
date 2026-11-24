// Package selfupdate lets the Windows service replace itself with a newer release, without update.ps1.
//
// The steps, all inside kinora.exe:
//
//  1. Prepare (server still running): download kinora-windows-amd64.zip and its .sha256, check the hash,
//     unpack into <install>\update\stage and check the version of the new kinora.exe.
//  2. RequestRestart: the server shuts down gracefully (the database is closed).
//  3. Swap: copy kinora.db* to <install>\update\backup\db, move every file the package replaces to
//     <install>\update\backup (Windows lets a running exe be renamed, only not deleted), move the new files
//     into place and write the marker <install>\update\pending.json. The process then exits as a failure and
//     the service's recovery actions start the new kinora.exe.
//  4. Guard, at every service start: with a marker present the new version is on probation. It counts the
//     starts; on the third start, or five minutes after the swap, it puts the old files and database back and
//     exits, so the old version starts again. A start that does not serve within the probation time exits too.
//  5. Confirm: the new version has served for a while; the marker goes and update.log gets "Sikeres frissítés".
//
// update.log keeps the format of update.ps1 ("2006-01-02 15:04:05  message"), so the dashboard reads the
// outcome of either the same way.
package selfupdate

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

const (
	// Asset is the release file a Windows install updates from.
	Asset = "kinora-windows-amd64.zip"
	// ProbationTime is how long a freshly swapped version may take to start serving.
	ProbationTime = 3 * time.Minute
	// maxStarts: on this start of a version still on probation the old one is put back.
	maxStarts   = 3
	rollbackAge = 5 * time.Minute
	maxUnpacked = 2 << 30 // the zip is ~100 MB; refuse anything absurd
)

// ErrRestart is what the server's run loop returns when it stopped for an update.
var ErrRestart = errors.New("restart for update")

var (
	mu         sync.Mutex
	installDir string // "" = self-update is not possible in this process
	stagedTag  string // the release Prepare staged
	restart    = make(chan struct{}, 1)
)

// Enable turns self-update on for this process: the Windows service calls it with the folder of kinora.exe.
func Enable(install string) {
	mu.Lock()
	installDir = install
	mu.Unlock()
}

// Enabled reports whether this process can update itself.
func Enabled() bool { return InstallDir() != "" }

// InstallDir is the folder of kinora.exe when self-update is on.
func InstallDir() string {
	mu.Lock()
	defer mu.Unlock()
	return installDir
}

// RequestRestart asks the run loop to shut down for the swap.
func RequestRestart() {
	select {
	case restart <- struct{}{}:
	default:
	}
}

// Restart is watched by the run loop: a value arrives when an update is staged.
func Restart() <-chan struct{} { return restart }

// Log appends one line to <install>\update.log in update.ps1's format.
func Log(install, msg string) {
	f, err := os.OpenFile(filepath.Join(install, "update.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "%s  %s\r\n", time.Now().Format("2006-01-02 15:04:05"), msg)
}

func updateDir(install string) string { return filepath.Join(install, "update") }
func stageDir(install string) string  { return filepath.Join(updateDir(install), "stage") }
func backupDir(install string) string { return filepath.Join(updateDir(install), "backup") }
func markerPath(install string) string {
	return filepath.Join(updateDir(install), "pending.json")
}

// Release is what Prepare downloads.
type Release struct {
	Tag    string // v1.2.3
	ZipURL string
	SumURL string
}

// CheckVersion runs the unpacked kinora.exe and checks that it is the expected release (tests replace it).
var CheckVersion = func(ctx context.Context, exe, tag string) error {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, exe, "version").Output()
	if err != nil {
		return fmt.Errorf("az új kinora.exe nem indul: %w", err)
	}
	if semver(string(out)) != semver(tag) || semver(tag) == "" {
		return fmt.Errorf("a csomagban lévő kinora.exe verziója váratlan: %s", strings.TrimSpace(string(out)))
	}
	return nil
}

var semverRe = regexp.MustCompile(`\d+\.\d+\.\d+`)

func semver(s string) string { return semverRe.FindString(s) }

// Prepare downloads, checks and unpacks rel into <install>\update\stage. The running server is not touched.
func Prepare(ctx context.Context, hc *http.Client, install string, rel Release) error {
	if err := os.RemoveAll(stageDir(install)); err != nil {
		return fmt.Errorf("a régi előkészítés nem törölhető: %w", err)
	}
	if err := os.MkdirAll(updateDir(install), 0o755); err != nil {
		return err
	}
	zipPath := filepath.Join(updateDir(install), Asset)
	defer os.Remove(zipPath)
	sum, err := download(ctx, hc, rel.ZipURL, zipPath)
	if err != nil {
		return fmt.Errorf("a letöltés nem sikerült: %w", err)
	}
	want, err := fetchSum(ctx, hc, rel.SumURL)
	if err != nil {
		return fmt.Errorf("az ellenőrzőösszeg nem tölthető le: %w", err)
	}
	if !strings.EqualFold(want, sum) {
		return errors.New("SHA-256 eltérés, a frissítés megszakítva.")
	}
	if err := unzip(zipPath, stageDir(install)); err != nil {
		os.RemoveAll(stageDir(install))
		return fmt.Errorf("a csomag nem bontható ki: %w", err)
	}
	if err := CheckVersion(ctx, filepath.Join(stageDir(install), "kinora.exe"), rel.Tag); err != nil {
		os.RemoveAll(stageDir(install))
		return err
	}
	mu.Lock()
	stagedTag = rel.Tag
	mu.Unlock()
	return nil
}

// StagedTag is the release the last successful Prepare of this process staged.
func StagedTag() string {
	mu.Lock()
	defer mu.Unlock()
	return stagedTag
}

func get(ctx context.Context, hc *http.Client, url string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "kinora-updater")
	resp, err := hc.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("HTTP %s", resp.Status)
	}
	return resp, nil
}

// download saves url to path and returns its SHA-256 (hex).
func download(ctx context.Context, hc *http.Client, url, path string) (string, error) {
	resp, err := get(ctx, hc, url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	f, err := os.Create(path)
	if err != nil {
		return "", err
	}
	h := sha256.New()
	_, err = io.Copy(io.MultiWriter(f, h), io.LimitReader(resp.Body, maxUnpacked))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// fetchSum reads "<hex>  <name>" and returns the hex.
func fetchSum(ctx context.Context, hc *http.Client, url string) (string, error) {
	resp, err := get(ctx, hc, url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if err != nil {
		return "", err
	}
	b = []byte(strings.TrimPrefix(string(b), "\ufeff"))
	fields := strings.Fields(string(b))
	if len(fields) == 0 || len(fields[0]) != 64 {
		return "", errors.New("hibás ellenőrzőösszeg-fájl")
	}
	if _, err := hex.DecodeString(fields[0]); err != nil {
		return "", errors.New("hibás ellenőrzőösszeg-fájl")
	}
	return fields[0], nil
}

// unzip extracts src into dst, refusing paths that would leave dst.
func unzip(src, dst string) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer r.Close()
	var total int64
	for _, f := range r.File {
		name := strings.ReplaceAll(f.Name, "\\", "/")
		if !fs.ValidPath(strings.TrimSuffix(name, "/")) {
			return fmt.Errorf("érvénytelen útvonal a csomagban: %q", f.Name)
		}
		target := filepath.Join(dst, filepath.FromSlash(name))
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		total += int64(f.UncompressedSize64)
		if total > maxUnpacked {
			return errors.New("a csomag túl nagy")
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		if err := extract(f, target); err != nil {
			return err
		}
	}
	if _, err := os.Stat(filepath.Join(dst, "kinora.exe")); err != nil {
		return errors.New("a csomagban nincs kinora.exe")
	}
	return nil
}

func extract(f *zip.File, target string) error {
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
	if err != nil {
		return err
	}
	_, err = io.Copy(out, io.LimitReader(rc, maxUnpacked))
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	return err
}

// Staged reports whether Prepare left a package ready to swap in.
func Staged(install string) bool {
	_, err := os.Stat(filepath.Join(stageDir(install), "kinora.exe"))
	return err == nil
}

// marker is <install>\update\pending.json: the swap that is still on probation.
type marker struct {
	From    string    `json:"from"`
	To      string    `json:"to"`
	Started time.Time `json:"started"`
	Starts  int       `json:"starts"`
	Files   []moved   `json:"files"`
}

type moved struct {
	Path   string `json:"path"`    // relative to the install folder
	HadOld bool   `json:"had_old"` // an older copy waits in update\backup
}

func readMarker(install string) (*marker, error) {
	b, err := os.ReadFile(markerPath(install))
	if err != nil {
		return nil, err
	}
	var m marker
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

func writeMarker(install string, m *marker) error {
	b, _ := json.MarshalIndent(m, "", "  ")
	tmp := markerPath(install) + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, markerPath(install))
}

// Swap backs up the database and the program files and moves the staged package into place. It runs after
// the server has stopped. On failure everything it moved is put back.
func Swap(install, data, from, to string) error {
	if !Staged(install) {
		return errors.New("nincs előkészített frissítés")
	}
	bk := backupDir(install)
	if err := os.RemoveAll(bk); err != nil {
		return fmt.Errorf("a régi mentés nem törölhető: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(bk, "db"), 0o755); err != nil {
		return err
	}
	dbs, _ := filepath.Glob(filepath.Join(data, "kinora.db*"))
	for _, p := range dbs {
		if err := copyFile(p, filepath.Join(bk, "db", filepath.Base(p))); err != nil {
			return fmt.Errorf("az adatbázis mentése nem sikerült: %w", err)
		}
	}
	var rels []string
	stage := stageDir(install)
	err := filepath.WalkDir(stage, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, err := filepath.Rel(stage, p)
		rels = append(rels, rel)
		return err
	})
	if err != nil {
		return err
	}
	m := &marker{From: from, To: to, Started: time.Now().UTC()}
	for _, rel := range rels {
		dest := filepath.Join(install, rel)
		mv := moved{Path: filepath.ToSlash(rel)}
		if _, err := os.Lstat(dest); err == nil {
			if err := os.MkdirAll(filepath.Dir(filepath.Join(bk, rel)), 0o755); err == nil {
				err = os.Rename(dest, filepath.Join(bk, rel))
			}
			if err != nil {
				putBack(install, m.Files)
				return fmt.Errorf("%s nem cserélhető: %w", rel, err)
			}
			mv.HadOld = true
		}
		m.Files = append(m.Files, mv) // before the move: putBack must also undo a half-done step
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err == nil {
			err = os.Rename(filepath.Join(stage, rel), dest)
		}
		if err != nil {
			putBack(install, m.Files)
			return fmt.Errorf("%s nem másolható a helyére: %w", rel, err)
		}
	}
	if err := writeMarker(install, m); err != nil {
		putBack(install, m.Files)
		return err
	}
	os.RemoveAll(stage)
	Log(install, "Mentés kész: "+bk)
	return nil
}

// putBack restores the program files listed in files from update\backup. A file it cannot delete (the
// running exe) is moved to update\trash, which the next start clears.
func putBack(install string, files []moved) error {
	var first error
	trash := filepath.Join(updateDir(install), "trash", time.Now().Format("20060102150405"))
	for i := len(files) - 1; i >= 0; i-- {
		rel := filepath.FromSlash(files[i].Path)
		dest := filepath.Join(install, rel)
		if _, err := os.Lstat(dest); err == nil {
			if err := os.Remove(dest); err != nil {
				os.MkdirAll(filepath.Dir(filepath.Join(trash, rel)), 0o755)
				if err := os.Rename(dest, filepath.Join(trash, rel)); err != nil && first == nil {
					first = err
				}
			}
		}
		if files[i].HadOld {
			if err := os.Rename(filepath.Join(backupDir(install), rel), dest); err != nil && first == nil {
				first = err
			}
		}
	}
	return first
}

// restoreDB puts the database copies of update\backup\db back. The server must not have it open.
func restoreDB(install, data string) error {
	saved, _ := filepath.Glob(filepath.Join(backupDir(install), "db", "kinora.db*"))
	if len(saved) == 0 {
		return nil // there was no database before the update
	}
	cur, _ := filepath.Glob(filepath.Join(data, "kinora.db*"))
	for _, p := range cur {
		if err := os.Remove(p); err != nil {
			return err
		}
	}
	for _, p := range saved {
		if err := copyFile(p, filepath.Join(data, filepath.Base(p))); err != nil {
			return err
		}
	}
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	_, err = io.Copy(out, in)
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	return err
}

// Guard runs at every service start, before the database is opened. It reports whether the process must
// exit right away (it has just put the previous version back, which the service manager starts instead), and
// whether this start is on probation (then Confirm must follow within ProbationTime).
func Guard(install, data, running string) (exit, probation bool) {
	os.RemoveAll(filepath.Join(updateDir(install), "trash"))
	m, err := readMarker(install)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			os.Remove(markerPath(install)) // unreadable: nothing sensible to roll back to
		}
		return false, false
	}
	if semver(running) != semver(m.To) {
		// not the version that was swapped in (put back by hand, or reinstalled): no probation
		os.Remove(markerPath(install))
		return false, false
	}
	m.Starts++
	if m.Starts < maxStarts && time.Since(m.Started) < rollbackAge {
		if writeMarker(install, m) != nil {
			return false, false
		}
		return false, true
	}
	Log(install, fmt.Sprintf("HIBA: a(z) %s nem indult el rendben – visszaállás erre: %s", m.To, m.From))
	err = putBack(install, m.Files)
	if err == nil {
		err = restoreDB(install, data)
	}
	if err != nil {
		Log(install, "HIBA: a visszaállás nem sikerült: "+err.Error())
		os.Remove(markerPath(install))
		return false, false // keep running what is there rather than loop
	}
	os.Remove(markerPath(install))
	Log(install, "Visszaállítva: "+m.From)
	return true, false
}

// Pending reports whether a swapped-in version is still on probation.
func Pending(install string) bool {
	_, err := os.Stat(markerPath(install))
	return err == nil
}

// Confirm ends the probation: the new version serves.
func Confirm(install string) {
	m, err := readMarker(install)
	if err != nil {
		return
	}
	os.Remove(markerPath(install))
	Log(install, "Sikeres frissítés: kinora "+m.To)
}
