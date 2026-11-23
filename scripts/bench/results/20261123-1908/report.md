```
date (UTC):   2026-11-23 19:08
kinora:       6990e86 (+ uncommitted changes)
jellyfin:     jellyfin/jellyfin:12.4.20261026-211555
docker:       28.4.0, Docker Desktop, kernel 6.6.87.2-microsoft-standard-WSL2, 12 CPUs, 8163381248 bytes visible to Docker
limits:       CPUs 0-3 (cpuset) and 4g RAM per server; load generator on CPUs 4-11
library:      5000 films + 500 series × 2 seasons × 5 episodes (2 s clips) + 2 one-minute 1080p files
runs:         scans ×3, API 100× per scenario, load 16 clients × 60 s, playback 5× per mode
host CPU:     Intel Core i5-1235U (2 P + 8 E cores, 12 threads), 15.7 GB RAM, Windows 11 Pro + Docker Desktop (WSL2), laptop on mains power
```

### Szkennelés

| | első szkennelés (s) | második szkennelés (s) | filmek / sorozatok / epizódok |
|---|---|---|---|
| kinora | 565.5 (560.6–653.9) | 0.3 (0.3–0.3) | 5002 / 500 / 5000 |
| jellyfin | 845.5 (843.5–1041.8) | 21.8 (19.0–42.7) | 5002 / 500 / 5000 |

Medián, zárójelben a legkisebb–legnagyobb érték, ismétlésenként friss konfigurációval.

### Erőforrás

| | üresjárat: CPU | üresjárat: RAM | terhelés alatt: CPU (átlag / csúcs) | terhelés alatt: RAM (csúcs) |
|---|---|---|---|---|
| kinora | 0.4% | 103 MiB | 394% / 400% | 167 MiB |
| jellyfin | 0.1% | 472 MiB | 353% / 360% | 488 MiB |

A CPU-érték a `docker stats` szerint: 100% = egy teljes mag.

### API-válaszidők (egy kliens, egymás után)

| | főoldal p50 / p95 (ms) | könyvtár, 60 elem p50 / p95 (ms) | keresés p50 / p95 (ms) |
|---|---|---|---|
| kinora | 29.5 / 38.1 | 7.2 / 9.1 | 9.6 / 11.9 |
| jellyfin | 127.4 / 165.2 | 59.7 / 88.9 | 86.0 / 118.6 |

### Terhelés

| | kliensek | művelet/s | főoldal p95 (ms) | könyvtár p95 (ms) | keresés p95 (ms) | hibák |
|---|---|---|---|---|---|---|
| kinora | 16 | 162 | 307 | 104 | 108 | 0 |
| jellyfin | 16 | 17 | 1648 | 1362 | 1502 | 0 |

Egy művelet egy forgatókönyv: a főoldal a Kinorában 1, a Jellyfinben 5 párhuzamos HTTP-kérés (amit a webes
kliense is küld), a könyvtároldal és a keresés mindkettőben 1.

### Lejátszás indulása

| | direct play: első 1 MiB, p50 / p95 (ms) | transcode: első HLS-szegmens, p50 / p95 (ms) |
|---|---|---|
| kinora | 4.7 / 8.4 | 1994.7 / 2000.3 |
| jellyfin | 74.8 / 107.6 | 3017.8 / 4155.8 |
