package server

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLastUpdateRun(t *testing.T) {
	for _, c := range []struct {
		name, log string
		want      *updateRun
	}{
		{"none", "", nil},
		{"success", "\uFEFF2026-11-20 04:00:01  Frissítés: kinora v1.0.16 → v1.0.17\n2026-11-20 04:00:09  Mentés kész: C:\\x\\backup\n2026-11-20 04:00:20  Sikeres frissítés: kinora v1.0.17\n",
			&updateRun{At: "2026-11-20 04:00:20", OK: true, Line: "Sikeres frissítés: kinora v1.0.17"}},
		{"rollback keeps the first error", "2026-11-20 04:00:01  Frissítés: kinora v1.0.17 → v1.0.18\n2026-11-20 04:01:40  Az új verzió nem válaszol a 8096 porton – visszaállás erre: kinora v1.0.17\n2026-11-20 04:02:10  Visszaállítva: kinora v1.0.17\n",
			&updateRun{At: "2026-11-20 04:01:40", Line: "Az új verzió nem válaszol a 8096 porton – visszaállás erre: kinora v1.0.17"}},
		{"later success wins", "2026-11-19 04:00:01  Frissítés: a → b\n2026-11-19 04:00:05  HIBA: SHA-256 eltérés, a frissítés megszakítva.\n2026-11-20 04:00:01  Frissítés: a → b\n2026-11-20 04:00:30  Sikeres frissítés: b\n",
			&updateRun{At: "2026-11-20 04:00:30", OK: true, Line: "Sikeres frissítés: b"}},
		{"later failure wins", "2026-11-19 04:00:30  Sikeres frissítés: b\n2026-11-20 04:00:05  HIBA: A Kinora szolgáltatás nincs telepítve (install.ps1).\n",
			&updateRun{At: "2026-11-20 04:00:05", Line: "HIBA: A Kinora szolgáltatás nincs telepítve (install.ps1)."}},
	} {
		dir := t.TempDir()
		if c.log != "" {
			os.WriteFile(filepath.Join(dir, "update.log"), []byte(c.log), 0o644)
		}
		got := lastUpdateRun(dir)
		if (got == nil) != (c.want == nil) || got != nil && *got != *c.want {
			t.Errorf("%s: got %+v, want %+v", c.name, got, c.want)
		}
	}
}

func TestApplyOutsideWindows(t *testing.T) {
	t.Setenv("KINORA_INSTALL", "docker")
	rc := newReleaseChecker()
	if ok, reason := rc.Apply(); ok || reason == "" {
		t.Fatalf("a docker install must not start update.ps1: %v %q", ok, reason)
	}
	if st := rc.withApply(releaseInfo{}); st.CanApply {
		t.Fatal("can_apply outside Windows")
	}
}
