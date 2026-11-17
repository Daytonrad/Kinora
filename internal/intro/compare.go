package intro

import (
	"math/bits"
	"sort"
)

// Matching thresholds (Chromaprint items are 32-bit words).
const (
	MaxBitErrors = 10 // an item "matches" when at most this many of its 32 bits differ
	MaxGapItems  = 8  // tolerate up to ~1 s of non-matching items inside a run
	MinDensity   = 0.8

	// Edge refinement: inside a real match items differ in ~1–8 bits, across the
	// boundary 9–19. The run's ends are trimmed until a 4-item window averages <= 7.
	edgeWin  = 4
	edgeMean = 7.0
	// Chromaprint items lag the audio by about 7 items (≈0.87 s: the classifier
	// filters span the following frames); measured with the generated test series.
	lagItems = 7
)

// Match is the longest common stretch of two fingerprints, as item indexes
// (start inclusive, end exclusive) in a and in b.
type Match struct {
	A0, A1, B0, B1 int
	Hits           int // matching items inside the run
}

// Len is the match length in items.
func (m Match) Len() int { return m.A1 - m.A0 }

// Longest finds the longest stretch where a and b agree at some time offset:
// for every shift it walks the overlap, counting an item as matching when the
// bit-error count is at most maxBits, and joins matches separated by at most
// maxGap items. Runs shorter than minLen items are ignored.
func Longest(a, b []uint32, maxBits, maxGap, minLen int) (Match, bool) {
	var best Match
	found := false
	for s := -(len(b) - 1); s < len(a); s++ {
		i0 := 0
		if s > 0 {
			i0 = s
		}
		i1 := len(a)
		if len(b)+s < i1 {
			i1 = len(b) + s
		}
		runStart, last, hits := -1, -1, 0
		flush := func() {
			if runStart >= 0 {
				n := last - runStart + 1
				// a real shared stretch matches almost everywhere; chance alignments of
				// unrelated audio are sparse hits chained through the gap tolerance
				if n >= minLen && n > best.Len() && float64(hits) >= MinDensity*float64(n) {
					best = Match{A0: runStart, A1: last + 1, B0: runStart - s, B1: last + 1 - s, Hits: hits}
					found = true
				}
			}
		}
		for i := i0; i < i1; i++ {
			if bits.OnesCount32(a[i]^b[i-s]) <= maxBits {
				if runStart < 0 || i-last > maxGap+1 {
					flush()
					runStart, hits = i, 0
				}
				last = i
				hits++
			}
		}
		flush()
	}
	if found {
		best = refine(a, b, best)
	}
	return best, found
}

func refine(a, b []uint32, m Match) Match {
	s := m.A0 - m.B0
	mean := func(from int) float64 {
		t := 0
		for i := from; i < from+edgeWin; i++ {
			t += bits.OnesCount32(a[i] ^ b[i-s])
		}
		return float64(t) / edgeWin
	}
	for m.A1-m.A0 > 2*edgeWin && mean(m.A0) > edgeMean {
		m.A0++
	}
	for m.A1-m.A0 > 2*edgeWin && mean(m.A1-edgeWin) > edgeMean {
		m.A1--
	}
	m.B0, m.B1 = m.A0-s, m.A1-s
	return m
}

// itemTime converts an item index of a fingerprint window to file time, correcting
// the fingerprint lag (except at the very start of a file, where 0 is the true start).
func itemTime(offset float64, idx int) float64 {
	if offset == 0 && idx <= 1 {
		return 0
	}
	return offset + float64(idx+lagItems)*ItemSec
}

// Episode is one episode's fingerprint window; Offset is the file time (seconds)
// of the first fingerprint item.
type Episode struct {
	ID     int64
	FP     []uint32
	Offset float64
}

// Found is a detected common segment of one episode, in file time.
type Found struct {
	Start, End float64
	Confidence float64 // share of this episode's comparisons that agree on the segment
	Votes      int
}

type cand struct{ start, end float64 }

// Common finds, for each episode, the segment it shares with other episodes of the
// season (intro or credits). Each episode is compared with up to `partners` others;
// the per-episode candidates vote, and the largest cluster (starts and ends within
// 2 s) wins. Segments outside [minSec, maxSec] are dropped.
func Common(eps []Episode, minSec, maxSec float64, partners int) map[int64]Found {
	minLen := int(minSec / ItemSec)
	cands := make([][]cand, len(eps))
	comps := make([]int, len(eps))
	for i := range eps {
		for k := 1; k <= partners && i+k < len(eps); k++ {
			j := i + k
			comps[i]++
			comps[j]++
			m, ok := Longest(eps[i].FP, eps[j].FP, MaxBitErrors, MaxGapItems, minLen)
			if !ok {
				continue
			}
			ai := cand{itemTime(eps[i].Offset, m.A0), itemTime(eps[i].Offset, m.A1)}
			bj := cand{itemTime(eps[j].Offset, m.B0), itemTime(eps[j].Offset, m.B1)}
			if d := ai.end - ai.start; d < minSec || d > maxSec {
				continue
			}
			cands[i] = append(cands[i], ai)
			cands[j] = append(cands[j], bj)
		}
	}
	out := map[int64]Found{}
	for i, cs := range cands {
		if len(cs) == 0 {
			continue
		}
		var bestCluster []cand
		for _, c := range cs {
			var cl []cand
			for _, o := range cs {
				if abs(o.start-c.start) <= 2 && abs(o.end-c.end) <= 2 {
					cl = append(cl, o)
				}
			}
			if len(cl) > len(bestCluster) || (len(cl) == len(bestCluster) && span(cl) > span(bestCluster)) {
				bestCluster = cl
			}
		}
		conf := float64(len(bestCluster)) / float64(comps[i])
		// one agreeing pair is enough only when the episode had a single comparison
		if len(bestCluster) < 2 && comps[i] > 1 && conf < 0.5 {
			continue
		}
		st, en := make([]float64, len(bestCluster)), make([]float64, len(bestCluster))
		for k, c := range bestCluster {
			st[k], en[k] = c.start, c.end
		}
		out[eps[i].ID] = Found{Start: median(st), End: median(en), Confidence: conf, Votes: len(bestCluster)}
	}
	return out
}

// Snippet cuts the items of [from, to) (file time) out of a fingerprint window that
// starts at offset, undoing the fingerprint lag.
func Snippet(fp []uint32, offset, from, to float64) []uint32 {
	i0 := int((from-offset)/ItemSec+0.5) - lagItems
	i1 := int((to-offset)/ItemSec+0.5) - lagItems
	if from <= offset {
		i0 = 0
	}
	if i0 < 0 {
		i0 = 0
	}
	if i1 > len(fp) {
		i1 = len(fp)
	}
	if i1 <= i0 {
		return nil
	}
	return fp[i0:i1]
}

// Locate finds a snippet (cut with Snippet, e.g. a hand-marked intro) inside another
// episode's fingerprint window; it returns the snippet's start in that episode (file
// time) and the share of the snippet that matched.
func Locate(snippet []uint32, ep Episode) (start float64, coverage float64, ok bool) {
	if len(snippet) == 0 {
		return 0, 0, false
	}
	m, found := Longest(snippet, ep.FP, MaxBitErrors, MaxGapItems, len(snippet)/2)
	if !found {
		return 0, 0, false
	}
	return itemTime(ep.Offset, m.B0-m.A0), float64(m.Len()) / float64(len(snippet)), true
}

func abs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

func span(cs []cand) float64 {
	s := 0.0
	for _, c := range cs {
		s += c.end - c.start
	}
	return s
}

func median(v []float64) float64 {
	sort.Float64s(v)
	n := len(v)
	if n%2 == 1 {
		return v[n/2]
	}
	return (v[n/2-1] + v[n/2]) / 2
}
