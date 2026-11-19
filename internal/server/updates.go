package server

// Release check for the admin dashboard: the server asks GitHub for the latest published release (not the main
// branch) at most every few hours, in the background, and the dashboard shows whether a newer one exists.
// Nothing is downloaded or installed here; the admin updates the way their install mode updates.

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	releaseRepo     = "ScotyLucas/Kinora"
	releaseCheckTTL = 6 * time.Hour
	releaseRetryTTL = 30 * time.Minute // after a failed check
	releaseForceGap = time.Minute      // between two "check now" clicks: GitHub allows 60 anonymous requests an hour
)

// ReleaseURL is the GitHub API endpoint asked for the latest release; tests point it at a local server.
var ReleaseURL = "https://api.github.com/repos/" + releaseRepo + "/releases/latest"

type releaseInfo struct {
	Current   string `json:"current"`
	Latest    string `json:"latest,omitempty"`
	URL       string `json:"url,omitempty"`
	Published int64  `json:"published_at,omitempty"`
	Available bool   `json:"available"`
	CheckedAt int64  `json:"checked_at,omitempty"`
	Error     bool   `json:"error,omitempty"`
	Disabled  bool   `json:"disabled,omitempty"` // dev build or KINORA_UPDATE_CHECK=0
	Mode      string `json:"mode"`               // install mode, so the UI can say how to update
}

type releaseChecker struct {
	mu      sync.Mutex
	hc      *http.Client
	info    releaseInfo
	next    time.Time
	last    time.Time // when the last check started
	running bool
}

func newReleaseChecker() *releaseChecker {
	return &releaseChecker{hc: &http.Client{Timeout: 15 * time.Second}}
}

var semverRe = regexp.MustCompile(`(\d+)\.(\d+)\.(\d+)`)

// parseSemver reads the first x.y.z in s ("v1.2.3", "kinora 1.2.3-rc1"); ok is false for "dev", "edge-…" and the like.
func parseSemver(s string) (v [3]int, ok bool) {
	m := semverRe.FindStringSubmatch(s)
	if m == nil {
		return v, false
	}
	for i := range v {
		v[i], _ = strconv.Atoi(m[i+1])
	}
	return v, true
}

func semverLess(a, b [3]int) bool {
	for i := range a {
		if a[i] != b[i] {
			return a[i] < b[i]
		}
	}
	return false
}

func updateCheckDisabled() bool {
	switch os.Getenv("KINORA_UPDATE_CHECK") {
	case "0", "false", "off", "no":
		return true
	}
	return false
}

// Status returns the last known result and starts a background check when it is stale. It never blocks on the network.
func (rc *releaseChecker) Status(ctx context.Context) releaseInfo {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	info := rc.info
	info.Current, info.Mode = Version, installMode()
	if _, ok := parseSemver(Version); !ok || updateCheckDisabled() {
		return releaseInfo{Current: Version, Mode: info.Mode, Disabled: true}
	}
	if !rc.running && time.Now().After(rc.next) {
		rc.running, rc.last = true, time.Now()
		go rc.check(ctx)
	}
	return withAvailable(info)
}

func withAvailable(info releaseInfo) releaseInfo {
	if info.Latest != "" {
		cur, _ := parseSemver(Version)
		lat, _ := parseSemver(info.Latest)
		info.Available = semverLess(cur, lat)
	}
	return info
}

// CheckNow asks GitHub right away and waits for the answer (the admin's "check now" button). Within releaseForceGap of
// the previous check, or while one is running, it just returns the last known result.
func (rc *releaseChecker) CheckNow(ctx context.Context) releaseInfo {
	rc.mu.Lock()
	if _, ok := parseSemver(Version); !ok || updateCheckDisabled() {
		rc.mu.Unlock()
		return releaseInfo{Current: Version, Mode: installMode(), Disabled: true}
	}
	if rc.running || time.Since(rc.last) < releaseForceGap {
		rc.mu.Unlock()
		return rc.Status(ctx)
	}
	rc.running, rc.last = true, time.Now()
	rc.mu.Unlock()
	rc.check(ctx)
	return rc.Status(ctx)
}

func (rc *releaseChecker) check(ctx context.Context) {
	latest, url, published, err := rc.fetch(ctx)
	rc.mu.Lock()
	defer rc.mu.Unlock()
	rc.running = false
	now := time.Now()
	if err != nil {
		rc.info.Error = true
		rc.next = now.Add(releaseRetryTTL)
		return
	}
	rc.info = releaseInfo{Latest: latest, URL: url, Published: published, CheckedAt: now.Unix()}
	rc.next = now.Add(releaseCheckTTL)
}

func (rc *releaseChecker) fetch(ctx context.Context) (tag, url string, published int64, err error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ReleaseURL, nil)
	if err != nil {
		return "", "", 0, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "kinora-server/"+Version)
	resp, err := rc.hc.Do(req)
	if err != nil {
		return "", "", 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", "", 0, fmt.Errorf("github: %s", resp.Status)
	}
	var rel struct {
		Tag        string    `json:"tag_name"`
		URL        string    `json:"html_url"`
		Draft      bool      `json:"draft"`
		Prerelease bool      `json:"prerelease"`
		Published  time.Time `json:"published_at"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&rel); err != nil {
		return "", "", 0, err
	}
	if _, ok := parseSemver(rel.Tag); !ok || rel.Draft || rel.Prerelease {
		return "", "", 0, fmt.Errorf("github: unexpected release %q", rel.Tag)
	}
	// only link to GitHub, never to whatever the response claims
	if !strings.HasPrefix(rel.URL, "https://github.com/") {
		rel.URL = "https://github.com/" + releaseRepo + "/releases"
	}
	var pub int64
	if !rel.Published.IsZero() {
		pub = rel.Published.Unix()
	}
	return rel.Tag, rel.URL, pub, nil
}
