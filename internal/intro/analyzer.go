package intro

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"sync"
	"time"

	"github.com/ScotyLucas/kinora/internal/store"
)

// Analyzer runs intro/credits detection in the background, one season at a time,
// at low priority, and pauses while anything is being transcoded or streamed.
type Analyzer struct {
	st  *store.Store
	fp  *Fingerprinter
	log *slog.Logger
	dir string // fingerprint cache

	IntroWindow   float64     // seconds from the start searched for the intro (default 600)
	CreditsWindow float64     // seconds before the end searched for the credits (default 240)
	Busy          func() bool // true while playback/transcoding runs: analysis waits
	// Settings is read before every season: detection on/off and the intro window (minutes).
	Settings func() (enabled bool, introWindowMin int)

	mu      sync.Mutex
	queue   []int64
	force   map[int64]bool
	kick    chan struct{}
	running string
	done    int
	lastErr string
}

// Status is shown on the admin page.
type Status struct {
	Backend   string             `json:"backend"`
	Available bool               `json:"available"`
	Queue     int                `json:"queue"`
	Running   string             `json:"running"`
	Done      int                `json:"done_seasons"`
	LastError string             `json:"last_error"`
	Stats     store.SegmentStats `json:"stats"`
}

// NewAnalyzer wires the detector to the store; cacheDir holds fingerprints.
func NewAnalyzer(st *store.Store, fp *Fingerprinter, cacheDir string, log *slog.Logger) *Analyzer {
	return &Analyzer{st: st, fp: fp, log: log, dir: filepath.Join(cacheDir, "fingerprints"),
		IntroWindow: 600, CreditsWindow: 240, force: map[int64]bool{}, kick: make(chan struct{}, 1)}
}

// Enqueue schedules seasons; force re-analyses even unchanged episodes.
func (a *Analyzer) Enqueue(seasons []int64, force bool) {
	a.mu.Lock()
	for _, id := range seasons {
		dup := false
		for _, q := range a.queue {
			dup = dup || q == id
		}
		if !dup {
			a.queue = append(a.queue, id)
		}
		if force {
			a.force[id] = true
		}
	}
	a.mu.Unlock()
	select {
	case a.kick <- struct{}{}:
	default:
	}
}

// Kick wakes the worker to look for new, unanalysed seasons (after a scan).
func (a *Analyzer) Kick() {
	select {
	case a.kick <- struct{}{}:
	default:
	}
}

// Status snapshot.
func (a *Analyzer) Status() Status {
	a.mu.Lock()
	defer a.mu.Unlock()
	return Status{Backend: a.fp.Backend(), Available: a.fp.Available(), Queue: len(a.queue), Running: a.running,
		Done: a.done, LastError: a.lastErr, Stats: a.st.SegmentStats()}
}

// Run processes the queue and periodically picks up seasons with new episodes.
func (a *Analyzer) Run(ctx context.Context) {
	if !a.fp.Available() {
		a.log.Info("intro detection disabled", "reason", a.fp.Backend())
		return
	}
	t := time.NewTicker(10 * time.Minute)
	defer t.Stop()
	for {
		a.drain(ctx)
		select {
		case <-ctx.Done():
			return
		case <-a.kick:
		case <-t.C:
		}
	}
}

func (a *Analyzer) drain(ctx context.Context) {
	tried := map[int64]bool{} // a season whose files keep failing is not retried until the next wake-up
	for ctx.Err() == nil {
		a.mu.Lock()
		if len(a.queue) == 0 {
			a.mu.Unlock()
			ids, err := a.st.SeasonsNeedingAnalysis(20)
			if err != nil {
				return
			}
			var fresh []int64
			for _, id := range ids {
				if !tried[id] {
					fresh = append(fresh, id)
				}
			}
			if len(fresh) == 0 {
				return
			}
			a.mu.Lock()
			a.queue = append(a.queue, fresh...)
		}
		if a.Settings != nil {
			if on, win := a.Settings(); !on {
				a.mu.Unlock()
				return
			} else if win > 0 {
				a.IntroWindow = float64(win) * 60
			}
		}
		id := a.queue[0]
		a.queue = a.queue[1:]
		tried[id] = true
		force := a.force[id]
		delete(a.force, id)
		a.mu.Unlock()
		for a.Busy != nil && a.Busy() && ctx.Err() == nil {
			select { // never compete with a viewer for CPU or disk
			case <-ctx.Done():
			case <-time.After(15 * time.Second):
			}
		}
		err := a.AnalyzeSeason(ctx, id, force)
		a.mu.Lock()
		a.running = ""
		a.done++
		if err != nil && !errors.Is(err, context.Canceled) {
			a.lastErr = err.Error()
			a.log.Warn("intro detection", "season", id, "err", err)
		}
		a.mu.Unlock()
	}
}

// AnalyzeSeason detects the intro and credits of every episode of a season.
func (a *Analyzer) AnalyzeSeason(ctx context.Context, seasonID int64, force bool) error {
	eps, err := a.st.SeasonEpisodes(seasonID)
	if err != nil {
		return err
	}
	if len(eps) == 0 {
		return nil
	}
	a.mu.Lock()
	a.running = eps[0].Label[:max(0, len(eps[0].Label)-3)]
	a.mu.Unlock()
	if len(eps) < 2 { // nothing to compare against yet; a new episode re-queues the season
		for _, e := range eps {
			a.st.MarkAnalysed(e.ID, e.Sig, "")
		}
		return nil
	}
	_ = force // fingerprints are cached per file signature; force only re-queues
	var intro, credits []Episode
	var errs []error
	type win struct{ start, dur float64 }
	creditWin := map[int64]win{}
	for _, e := range eps {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		iw := a.IntroWindow
		if e.RuntimeSec > 0 && e.RuntimeSec*0.45 < iw {
			iw = e.RuntimeSec * 0.45
		}
		fp, err := a.fp.Cached(ctx, a.dir, e.ID, e.Sig, e.Path, 0, iw)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", e.Label, err))
			a.st.MarkAnalysed(e.ID, e.Sig, err.Error())
			continue
		}
		intro = append(intro, Episode{ID: e.ID, FP: fp, Offset: 0})
		if e.RuntimeSec > 60 {
			cs := e.RuntimeSec - a.CreditsWindow
			if cs < e.RuntimeSec*0.55 {
				cs = e.RuntimeSec * 0.55
			}
			if cfp, err := a.fp.Cached(ctx, a.dir, e.ID, e.Sig, e.Path, cs, e.RuntimeSec-cs); err == nil {
				credits = append(credits, Episode{ID: e.ID, FP: cfp, Offset: cs})
				creditWin[e.ID] = win{cs, e.RuntimeSec - cs}
			}
		}
	}
	byID := map[int64]store.AnalysisEpisode{}
	for _, e := range eps {
		byID[e.ID] = e
	}
	foundIntro := Common(intro, 15, 150, 3)
	foundCredits := Common(credits, 10, a.CreditsWindow, 3)
	for _, e := range intro {
		a.st.ClearAutoSegments(e.ID, "intro")
		if f, ok := foundIntro[e.ID]; ok {
			a.st.SetAutoSegment(e.ID, store.Segment{Type: "intro", Start: round(f.Start), End: round(f.End), Confidence: f.Confidence})
		}
	}
	for _, e := range credits {
		a.st.ClearAutoSegments(e.ID, "credits")
		f, ok := foundCredits[e.ID]
		rt := byID[e.ID].RuntimeSec
		if !ok || f.End < rt-45 { // a shared stretch that is not at the end is not the credits
			continue
		}
		start := f.Start
		// snap to a fade to black just before the music starts
		if bl, err := a.fp.BlackRanges(ctx, byID[e.ID].Path, maxf(0, start-6), 8); err == nil {
			for _, b := range bl {
				if b[0] <= start+1 && b[1] >= start-6 {
					start = b[0]
					break
				}
			}
		}
		a.st.SetAutoSegment(e.ID, store.Segment{Type: "credits", Start: round(start), End: rt, Confidence: f.Confidence})
	}
	for _, e := range eps {
		a.st.MarkAnalysed(e.ID, e.Sig, "")
	}
	return errors.Join(errs...)
}

// Propagate copies a hand-marked segment to the other episodes of the season where
// the same fingerprint stretch is found. It returns how many episodes were updated.
func (a *Analyzer) Propagate(ctx context.Context, itemID int64, typ string) (int, error) {
	if !a.fp.Available() {
		return 0, errNoBackend
	}
	var seg *store.Segment
	stored, err := a.st.StoredSegments(itemID)
	if err != nil {
		return 0, err
	}
	for i := range stored {
		if stored[i].Type == typ && stored[i].Source == "manual" {
			seg = &stored[i]
		}
	}
	if seg == nil {
		return 0, fmt.Errorf("ehhez az epizódhoz nincs kézzel jelölt %s", typ)
	}
	seasons, err := a.st.AnalysisSeasonsOf(itemID)
	if err != nil || len(seasons) == 0 {
		return 0, fmt.Errorf("az elem nem epizód")
	}
	eps, err := a.st.SeasonEpisodes(seasons[0])
	if err != nil {
		return 0, err
	}
	// every episode (the marked one too) is fingerprinted over the same kind of window,
	// so the snippet and the targets share the item grid and the lag cancels out
	window := func(e store.AnalysisEpisode) (float64, float64) {
		if typ == "credits" && e.RuntimeSec > 0 {
			start := maxf(0, e.RuntimeSec-a.CreditsWindow-60)
			return start, e.RuntimeSec - start
		}
		dur := a.IntroWindow
		if e.RuntimeSec > 0 && e.RuntimeSec*0.6 < dur {
			dur = e.RuntimeSec * 0.6
		}
		if seg.End > dur && (e.RuntimeSec == 0 || seg.End < e.RuntimeSec) {
			dur = seg.End + 30
		}
		return 0, dur
	}
	var snippet []uint32
	for _, e := range eps {
		if e.ID == itemID {
			ws, wd := window(e)
			fp, err := a.fp.Cached(ctx, a.dir, e.ID, e.Sig, e.Path, ws, wd)
			if err != nil {
				return 0, err
			}
			snippet = Snippet(fp, ws, seg.Start, seg.End)
		}
	}
	if minSnippet := 10.0; float64(len(snippet))*ItemSec < minSnippet {
		return 0, fmt.Errorf("a jelölt szakasz túl rövid az összevetéshez (legalább 10 mp kell)")
	}
	n := 0
	for _, e := range eps {
		if e.ID == itemID || ctx.Err() != nil {
			continue
		}
		start, dur := window(e)
		fp, err := a.fp.Cached(ctx, a.dir, e.ID, e.Sig, e.Path, start, dur)
		if err != nil {
			continue
		}
		at, cov, ok := Locate(snippet, Episode{ID: e.ID, FP: fp, Offset: start})
		if !ok || cov < 0.8 {
			continue
		}
		g := store.Segment{Type: typ, Start: round(maxf(0, at)), End: round(at + seg.End - seg.Start), Confidence: cov}
		if typ == "credits" && e.RuntimeSec > 0 {
			g.End = e.RuntimeSec
		}
		if a.st.SetManualSegment(e.ID, g) == nil {
			n++
		}
	}
	return n, nil
}

func round(v float64) float64 { return float64(int(v*10+0.5)) / 10 }

func maxf(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
