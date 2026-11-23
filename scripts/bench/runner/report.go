package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// report reads a results directory written by bench.sh:
//
//	env.txt                         machine and versions (printed as is)
//	<target>-scan-<n>.json          runner scan, one per repetition
//	<target>-api.json, -load.json, -play.json
//	<target>-idle.txt               "<start unix> <end unix>" of the idle window
//	<target>-stats.csv              "<unix>;<CPU %>;<mem usage>" lines from docker stats
func report(dir string, w io.Writer) error {
	if dir == "" {
		return fmt.Errorf("-dir is required")
	}
	targets := []string{"kinora", "jellyfin"}
	p := func(format string, a ...any) { fmt.Fprintf(w, format, a...) }

	if b, err := os.ReadFile(filepath.Join(dir, "env.txt")); err == nil {
		p("```\n%s```\n\n", b)
	}

	// scans
	p("### Szkennelés\n\n| | első szkennelés (s) | második szkennelés (s) | filmek / sorozatok / epizódok |\n|---|---|---|---|\n")
	for _, t := range targets {
		files, _ := filepath.Glob(filepath.Join(dir, t+"-scan-*.json"))
		var first, second []float64
		var c counts
		for _, f := range files {
			var r scanResult
			if readJSON(f, &r) == nil {
				first, second, c = append(first, r.FirstScanS), append(second, r.SecondScanS), r.Counts
			}
		}
		if len(first) == 0 {
			continue
		}
		p("| %s | %s | %s | %d / %d / %d |\n", t, spread(first), spread(second), c.Movies, c.Series, c.Episodes)
	}
	p("\nMedián, zárójelben a legkisebb–legnagyobb érték, ismétlésenként friss konfigurációval.\n\n")

	// resources
	p("### Erőforrás\n\n| | üresjárat: CPU | üresjárat: RAM | terhelés alatt: CPU (átlag / csúcs) | terhelés alatt: RAM (csúcs) |\n|---|---|---|---|---|\n")
	for _, t := range targets {
		samples := readStats(filepath.Join(dir, t+"-stats.csv"))
		if len(samples) == 0 {
			continue
		}
		var idleS, idleE float64
		if b, err := os.ReadFile(filepath.Join(dir, t+"-idle.txt")); err == nil {
			fmt.Sscan(string(b), &idleS, &idleE)
		}
		var load loadResult
		readJSON(filepath.Join(dir, t+"-load.json"), &load)
		idle := window(samples, idleS, idleE)
		busy := window(samples, load.StartedUnix+5, load.EndedUnix-1)
		p("| %s | %.1f%% | %s | %.0f%% / %.0f%% | %s |\n", t, avg(idle, cpuOf), mib(avg(idle, memOf)),
			avg(busy, cpuOf), maxOf(busy, cpuOf), mib(maxOf(busy, memOf)))
	}
	p("\nA CPU-érték a `docker stats` szerint: 100%% = egy teljes mag.\n\n")

	// API
	p("### API-válaszidők (egy kliens, egymás után)\n\n| | főoldal p50 / p95 (ms) | könyvtár, 60 elem p50 / p95 (ms) | keresés p50 / p95 (ms) |\n|---|---|---|---|\n")
	for _, t := range targets {
		var r apiResult
		if readJSON(filepath.Join(dir, t+"-api.json"), &r) != nil {
			continue
		}
		p("| %s | %s | %s | %s |\n", t, pp(r.Scenarios["home"]), pp(r.Scenarios["library"]), pp(r.Scenarios["search"]))
	}
	p("\n### Terhelés\n\n| | kliensek | művelet/s | főoldal p95 (ms) | könyvtár p95 (ms) | keresés p95 (ms) | hibák |\n|---|---|---|---|---|---|---|\n")
	for _, t := range targets {
		var r loadResult
		if readJSON(filepath.Join(dir, t+"-load.json"), &r) != nil {
			continue
		}
		errs := 0
		for _, l := range r.Scenarios {
			errs += l.Errs
		}
		p("| %s | %d | %.0f | %.0f | %.0f | %.0f | %d |\n", t, r.Clients, r.ReqPerS,
			r.Scenarios["home"].P95ms, r.Scenarios["library"].P95ms, r.Scenarios["search"].P95ms, errs)
	}

	p("\nEgy művelet egy forgatókönyv: a főoldal a Kinorában 1, a Jellyfinben 5 párhuzamos HTTP-kérés (amit a webes\n" +
		"kliense is küld), a könyvtároldal és a keresés mindkettőben 1.\n")

	// playback
	p("\n### Lejátszás indulása\n\n| | direct play: első 1 MiB, p50 / p95 (ms) | transcode: első HLS-szegmens, p50 / p95 (ms) |\n|---|---|---|\n")
	for _, t := range targets {
		var r playResult
		if readJSON(filepath.Join(dir, t+"-play.json"), &r) != nil {
			continue
		}
		p("| %s | %s | %s |\n", t, pp(r.Direct), pp(r.Transcode))
	}
	return nil
}

func readJSON(path string, v any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

func spread(v []float64) string {
	s := append([]float64(nil), v...)
	sort.Float64s(s)
	med := s[len(s)/2]
	if len(s)%2 == 0 {
		med = (s[len(s)/2-1] + s[len(s)/2]) / 2
	}
	if len(s) == 1 {
		return fmt.Sprintf("%.1f", med)
	}
	return fmt.Sprintf("%.1f (%.1f–%.1f)", med, s[0], s[len(s)-1])
}

func pp(l latency) string {
	if l.N == 0 {
		return "–"
	}
	if l.Errs > 0 {
		return fmt.Sprintf("%.1f / %.1f (%d hiba)", l.P50ms, l.P95ms, l.Errs)
	}
	return fmt.Sprintf("%.1f / %.1f", l.P50ms, l.P95ms)
}

type sample struct{ at, cpu, memMiB float64 }

func cpuOf(s sample) float64 { return s.cpu }
func memOf(s sample) float64 { return s.memMiB }

func readStats(path string) []sample {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	var out []sample
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		parts := strings.Split(sc.Text(), ";")
		if len(parts) < 3 {
			continue
		}
		at, err1 := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
		cpu, err2 := strconv.ParseFloat(strings.TrimSuffix(strings.TrimSpace(parts[1]), "%"), 64)
		mem := parseMem(strings.TrimSpace(strings.Split(parts[2], "/")[0]))
		if err1 != nil || err2 != nil || mem < 0 {
			continue
		}
		out = append(out, sample{at, cpu, mem})
	}
	return out
}

// parseMem reads docker stats' "512.3MiB" style values into MiB (-1 when unreadable).
func parseMem(v string) float64 {
	for _, u := range []struct {
		suffix string
		mul    float64
	}{{"GiB", 1024}, {"MiB", 1}, {"KiB", 1.0 / 1024}, {"kB", 1.0 / 1024}, {"GB", 1000 * 1000 * 1000 / (1 << 20)},
		{"MB", 1000 * 1000 / (1 << 20)}, {"B", 1.0 / (1 << 20)}} {
		if strings.HasSuffix(v, u.suffix) {
			n, err := strconv.ParseFloat(strings.TrimSuffix(v, u.suffix), 64)
			if err != nil {
				return -1
			}
			return n * u.mul
		}
	}
	return -1
}

func window(s []sample, from, to float64) []sample {
	var out []sample
	for _, x := range s {
		if x.at >= from && x.at <= to {
			out = append(out, x)
		}
	}
	return out
}

func avg(s []sample, f func(sample) float64) float64 {
	if len(s) == 0 {
		return math.NaN()
	}
	var sum float64
	for _, x := range s {
		sum += f(x)
	}
	return sum / float64(len(s))
}

func maxOf(s []sample, f func(sample) float64) float64 {
	m := math.NaN()
	for _, x := range s {
		if v := f(x); math.IsNaN(m) || v > m {
			m = v
		}
	}
	return m
}

func mib(v float64) string {
	if math.IsNaN(v) {
		return "–"
	}
	return fmt.Sprintf("%.0f MiB", v)
}
