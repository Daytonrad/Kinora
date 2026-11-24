package server

// One-click update from the admin dashboard on a native Windows install. The server updates itself (see
// internal/selfupdate): it downloads and checks the release while it keeps serving, then stops, swaps the
// files and is started again by the service manager. The dashboard waits for the new version to answer.
// update.ps1 stays for the nightly scheduled task and for updating by hand; both write update.log.

import (
	"bufio"
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ScotyLucas/kinora/internal/selfupdate"
)

// applyWindow is how long one started update counts as running: a second click inside it is refused.
const applyWindow = 10 * time.Minute

// updateRun is the outcome of the last update.ps1 run, read from the last line of update.log.
type updateRun struct {
	At   string `json:"at"`
	OK   bool   `json:"ok"`
	Line string `json:"line"`
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
	install := selfupdate.InstallDir()
	if install == "" {
		return info
	}
	info.CanApply = true
	info.Applying = !rc.applied.IsZero() && time.Since(rc.applied) < applyWindow
	info.LastRun = lastUpdateRun(install)
	return info
}

// Apply starts the self-update in the background. It returns a short reason when it cannot.
func (rc *releaseChecker) Apply() (ok bool, reason string) {
	install := selfupdate.InstallDir()
	if install == "" {
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
	if rc.zipURL == "" || rc.sumURL == "" {
		return false, "a(z) " + rc.info.Latest + " kiadásban nincs Windows-csomag"
	}
	rel := selfupdate.Release{Tag: rc.info.Latest, ZipURL: rc.zipURL, SumURL: rc.sumURL}
	rc.applied = time.Now()
	selfupdate.Log(install, "Frissítés: kinora "+Version+" → "+rel.Tag)
	go rc.prepare(install, rel)
	return true, ""
}

// prepare downloads and stages rel, then asks the process to restart into it. A failure goes to update.log,
// where the dashboard shows it, and frees the button again.
func (rc *releaseChecker) prepare(install string, rel selfupdate.Release) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()
	if err := selfupdate.Prepare(ctx, &http.Client{}, install, rel); err != nil {
		selfupdate.Log(install, "HIBA: "+err.Error())
		rc.mu.Lock()
		rc.applied = time.Time{}
		rc.mu.Unlock()
		return
	}
	selfupdate.RequestRestart()
}
