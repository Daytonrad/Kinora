# Kinora vs Jellyfin benchmark

Reprodukálható mérés Docker Compose-zal: a két szerver ugyanazon a generált könyvtáron, ugyanazokkal a CPU- és
memóriakorlátokkal, egymás után (soha nem egyszerre) fut. Az eredmények és a módszer részletei:
[`docs/BENCHMARKS.md`](../../docs/BENCHMARKS.md).

```bash
scripts/bench/bench.sh              # mindkét szerver (kb. 1,5–2 óra a teljes könyvtárral)
scripts/bench/bench.sh kinora       # csak az egyik
MOVIES=200 SHOWS=20 REPS=1 LOAD_S=15 scripts/bench/bench.sh   # gyors próba
```

Kell hozzá: Docker Compose v2, bash (Linux, macOS vagy Git Bash Windowson), és legalább 8 CPU-szál (alapból a
szerverek a `0-3`, a terhelő program a `4-11` CPU-kat kapja; más gépen `BENCH_CPUSET` és `RUNNER_CPUSET`). A
többi beállítás a `bench.sh` elején.

| Fájl | Szerep |
|---|---|
| `compose.yml` | a két szerver, a könyvtárgenerátor (`gen`) és a mérőprogram (`runner`) |
| `gen-library.sh` | a tesztkönyvtár egy Docker-kötetbe (nem a host fájlrendszerére, hogy annak sebessége ne torzítson) |
| `runner/` | Go mérőprogram (csak standard könyvtár): beállítás, szkennelés, API, terhelés, lejátszás, jelentés |
| `bench.sh` | a teljes menet; a `docker stats` mintákat a host gyűjti |
| `results/<időpont>/` | nyers JSON, CPU/RAM-minták és a `report.md` |
