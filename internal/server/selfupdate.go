package server

// One-click update from the admin dashboard on a native Windows install. The server does not replace itself:
// it starts the same update.ps1 the nightly scheduled task runs (download, SHA-256 check, backup, swap, health
// check, rollback), as a detached process that outlives the service it stops. The dashboard then waits for the
// new version to answer.

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// applyWindow is how long one started update counts as running: a second click inside it is refused.
const applyWindow = 10 * time.Minute

// updateRun is the outcome of the last update.ps1 run, read from the last line of update.log.
type updateRun struct {
	At   string `json:"at"`
	OK   bool   `json:"ok"`
	Line string `json:"line"`
}

// installDir is the folder of the running kinora.exe (update.ps1 and update.log live there).
var installDir = func() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	if r, err := filepath.EvalSymlinks(exe); err == nil {
		exe = r
	}
	return filepath.Dir(exe)
}

// updateScript returns the path of update.ps1 when this install can update itself from the dashboard.
func updateScript() (string, bool) {
	if installMode() != "windows" {
		return "", false
	}
	dir := installDir()
	if dir == "" {
		return "", false
	}
	p := filepath.Join(dir, "update.ps1")
	if fi, err := os.Stat(p); err != nil || fi.IsDir() {
		return "", false
	}
	return p, true
}

// lastUpdateRun reads the final result line of update.log: success, failure or rollback.
func lastUpdateRun(dir string) *updateRun {
	f, err := os.Open(filepath.Join(dir, "update.log"))
	if err != nil {
		return nil
	}
	defer f.Close()
	// A run starts with "Frissítés: a → b" and ends with "Sikeres frissítés: …" or with its first error line
	// (the rollback lines after that only repeat it). An error before the start line is a run of its own.
	var last *updateRun
	open := false // inside a run that has not ended yet
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimPrefix(strings.TrimSpace(sc.Text()), "\ufeff")
		at, msg, ok := strings.Cut(line, "  ") // "2026-11-20 04:01:02  message"
		if !ok || len(at) != 19 {
			continue
		}
		switch {
		case strings.HasPrefix(msg, "Frissítés:"):
			open = true
		case strings.HasPrefix(msg, "Sikeres frissítés"):
			last, open = &updateRun{At: at, OK: true, Line: msg}, false
		case strings.HasPrefix(msg, "HIBA"), strings.Contains(msg, "visszaállás"):
			if open || last == nil || last.OK || !sameRun(last.At, at) {
				last, open = &updateRun{At: at, Line: msg}, false
			}
		}
	}
	return last
}

// sameRun: two log times close enough to belong to one run (a rollback takes at most a few minutes).
func sameRun(a, b string) bool {
	const layout = "2006-01-02 15:04:05"
	ta, err1 := time.Parse(layout, a)
	tb, err2 := time.Parse(layout, b)
	return err1 == nil && err2 == nil && tb.Sub(ta) < 5*time.Minute
}

// withApply fills in what the dashboard needs for the "update now" button.
func (rc *releaseChecker) withApply(info releaseInfo) releaseInfo {
	if _, ok := updateScript(); !ok {
		return info
	}
	info.CanApply = true
	info.Applying = !rc.applied.IsZero() && time.Since(rc.applied) < applyWindow
	info.LastRun = lastUpdateRun(installDir())
	return info
}

// Apply starts update.ps1 in the background. It returns a short reason when it cannot.
func (rc *releaseChecker) Apply() (ok bool, reason string) {
	script, can := updateScript()
	if !can {
		return false, "ez a telepítés nem frissíthető innen"
	}
	rc.mu.Lock()
	defer rc.mu.Unlock()
	if !rc.applied.IsZero() && time.Since(rc.applied) < applyWindow {
		return false, "a frissítés már fut"
	}
	if !withAvailable(rc.info).Available {
		return false, "nincs újabb kiadás"
	}
	if err := startDetached(script); err != nil {
		return false, "a frissítés nem indult el: " + err.Error()
	}
	rc.applied = time.Now()
	return true, ""
}
