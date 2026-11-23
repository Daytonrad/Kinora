package main

import (
	"bytes"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseMem(t *testing.T) {
	for in, want := range map[string]float64{"512.5MiB": 512.5, "1.5GiB": 1536, "2048KiB": 2, "0B": 0, "x": -1, "12MiBx": -1} {
		if got := parseMem(in); math.Abs(got-want) > 1e-9 {
			t.Errorf("parseMem(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestSummarize(t *testing.T) {
	var ds []time.Duration
	for i := 100; i >= 1; i-- {
		ds = append(ds, time.Duration(i)*time.Millisecond)
	}
	l := summarize(ds, 2)
	if l.N != 100 || l.Errs != 2 || l.P50ms != 51 || l.P95ms != 96 || l.P99ms != 100 || l.Mean != 50.5 {
		t.Fatalf("%+v", l)
	}
	if l := summarize(nil, 3); l.N != 0 || l.Errs != 3 {
		t.Fatalf("empty: %+v", l)
	}
}

func TestSpread(t *testing.T) {
	if s := spread([]float64{30, 10, 20}); s != "20.0 (10.0–30.0)" {
		t.Fatal(s)
	}
	if s := spread([]float64{4, 2}); s != "3.0 (2.0–4.0)" {
		t.Fatal(s)
	}
	if s := spread([]float64{7}); s != "7.0" {
		t.Fatal(s)
	}
}

func TestParseJFTime(t *testing.T) {
	for _, v := range []string{"2026-11-23T18:00:01.1234567Z", "2026-11-23T18:00:01.1234567", "2026-11-23T18:00:01+00:00"} {
		got, err := parseJFTime(v)
		if err != nil || got.UTC().Format("15:04:05") != "18:00:01" {
			t.Errorf("%s: %v %v", v, got, err)
		}
	}
}

func TestReport(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("env.txt", "machine: test\n")
	write("kinora-scan-1.json", `{"target":"kinora","first_scan_s":10,"second_scan_s":2,"counts":{"Movies":5002,"Series":500,"Episodes":5000}}`)
	write("kinora-scan-2.json", `{"target":"kinora","first_scan_s":12,"second_scan_s":3,"counts":{"Movies":5002,"Series":500,"Episodes":5000}}`)
	write("kinora-idle.txt", "100 200")
	write("kinora-load.json", `{"target":"kinora","clients":16,"duration_s":60,"requests":600,"req_per_s":10,"started_unix":300,"ended_unix":360,
		"scenarios":{"home":{"n":200,"p95_ms":40},"library":{"n":200,"p95_ms":30},"search":{"n":200,"p95_ms":20}}}`)
	write("kinora-stats.csv", "150;1.00%;100MiB / 4GiB\n160;3.00%;120MiB / 4GiB\n310;250.0%;300MiB / 4GiB\n320;150.0%;350MiB / 4GiB\nbad line\n")
	write("kinora-api.json", `{"target":"kinora","scenarios":{"home":{"n":100,"p50_ms":5,"p95_ms":8},"library":{"n":100,"p50_ms":3,"p95_ms":4},"search":{"n":100,"p50_ms":2,"p95_ms":3}}}`)
	write("kinora-play.json", `{"target":"kinora","direct":{"n":5,"p50_ms":12,"p95_ms":15},"transcode":{"n":5,"p50_ms":900,"p95_ms":1100,"errors":1}}`)
	var b bytes.Buffer
	if err := report(dir, &b); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	for _, want := range []string{
		"machine: test",
		"| kinora | 11.0 (10.0–12.0) | 2.5 (2.0–3.0) | 5002 / 500 / 5000 |",
		"| kinora | 2.0% | 110 MiB | 200% / 250% | 350 MiB |",
		"| kinora | 5.0 / 8.0 | 3.0 / 4.0 | 2.0 / 3.0 |",
		"| kinora | 16 | 10 | 40 | 30 | 20 | 0 |",
		"| kinora | 12.0 / 15.0 | 900.0 / 1100.0 (1 hiba) |",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in\n%s", want, out)
		}
	}
	if strings.Contains(out, "| jellyfin |") {
		t.Error("a target without results got a row")
	}
}
