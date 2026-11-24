# Benchmark: Kinora és Jellyfin

**Magyar** · [English summary](#english-summary)

Mérés: 2026-11-23, egyetlen gépen, egy futás (a szkennelés háromszor). A mérés megismételhető: `scripts/bench/`
([leírás](../scripts/bench/README.md)); a nyers adatok: [`scripts/bench/results/20261123-1908/`](../scripts/bench/results/20261123-1908/).

## Környezet

| | |
|---|---|
| Gép | Intel Core i5-1235U laptop (2 teljesítmény- + 8 hatékonysági mag, 12 szál), 15,7 GB RAM, hálózatról táplálva |
| Rendszer | Windows 11 Pro, Docker Desktop 28.4.0 (WSL2, kernel 6.6.87.2), a Dockernek 12 CPU és 7,6 GiB RAM |
| Korlátok | mindkét szerver ugyanazon a 4 CPU-n (`cpuset: 0-3`), 4 GB memóriakorláttal; a terhelő program a 4–11. CPU-n |
| Kinora | a `6990e86` commit és a még nem commitolt 6–7. fázis (varázsló, offline letöltés; a mért útvonalakat nem érinti) |
| Jellyfin | `jellyfin/jellyfin:12.4.20261026-211555` (12.4.0) |
| Könyvtár | 5000 film + 500 sorozat × 2 évad × 5 rész (2 másodperces 320×180 H.264/AAC klipek), plusz két 1 perces 1080p fájl a lejátszáshoz (H.264/AAC MP4 és HEVC/AC-3 MKV); összesen 10 002 videó, Docker-kötetben |

A két szerver soha nem futott egyszerre. A szerverek egymás után, ugyanazon a könyvtáron futottak.

## Eredmények

### Szkennelés

| | első szkennelés | második szkennelés (nincs változás) | beolvasva: filmek / sorozatok / részek |
|---|---|---|---|
| Kinora | **565,5 s** (560,6–653,9) | **0,3 s** | 5002 / 500 / 5000 |
| Jellyfin | 845,5 s (843,5–1041,8) | 21,8 s (19,0–42,7) | 5002 / 500 / 5000 |

Medián, zárójelben a legkisebb és a legnagyobb érték. Mindhárom ismétlés friss konfigurációval indult.

### Erőforrás

| | üresjárat: CPU | üresjárat: RAM | terhelés alatt: CPU (átlag / csúcs) | terhelés alatt: RAM (csúcs) |
|---|---|---|---|---|
| Kinora | 0,4% | **103 MiB** | 394% / 400% | **167 MiB** |
| Jellyfin | **0,1%** | 472 MiB | 353% / 360% | 488 MiB |

`docker stats` szerint: 100% = egy teljes mag. Az üresjárati értékek a szkennelés utáni háttérmunka lecsengése után,
60 másodpercen át mért átlagok.

### API-válaszidők (egy kliens, egymás után, 100 kérés)

| | főoldal p50 / p95 | könyvtár, 60 elem p50 / p95 | keresés p50 / p95 |
|---|---|---|---|
| Kinora | **29,5 / 38,1 ms** | **7,2 / 9,1 ms** | **9,6 / 11,9 ms** |
| Jellyfin | 127,4 / 165,2 ms | 59,7 / 88,9 ms | 86,0 / 118,6 ms |

### Terhelés (16 párhuzamos kliens, 60 másodperc)

| | művelet/s | főoldal p95 | könyvtár p95 | keresés p95 | hibák |
|---|---|---|---|---|---|
| Kinora | **162** | **307 ms** | **104 ms** | **108 ms** | 0 |
| Jellyfin | 17 | 1648 ms | 1362 ms | 1502 ms | 0 |

### Lejátszás indulása (5 ismétlés)

| | direct play: az első 1 MiB, p50 / p95 | átkódolás: az első HLS-szegmens, p50 / p95 |
|---|---|---|
| Kinora | **4,7 / 8,4 ms** | **1995 / 2000 ms** |
| Jellyfin | 74,8 / 107,6 ms | 3018 / 4156 ms |

## Értelmezés

- **Szkennelés:** az első szkennelés a Kinorában kb. 33%-kal rövidebb, a második (változás nélküli) kb. 70-szer
  rövidebb. Az első szkennelés mindkettőnél lassú, mert fájlonként ffprobe fut, és egy képkocka-előnézet készül.
- **Memória:** a Kinora üresjáratban kb. 4,6-szor, terhelés alatt kb. 2,9-szer kevesebb memóriát használ.
- **API:** egy kliensnél a Kinora válaszai 4–9-szer gyorsabbak. 16 kliensnél kb. 9-szer annyi műveletet szolgál ki,
  rövidebb várakozási időkkel. Terhelés alatt mindkét szerver kihasználja a 4 magot; a Kinora valamivel többet (394% a
  353%-kal szemben), de közben kb. 9-szer annyi munkát végez.
- **Lejátszás:** a direct play első megabájtja kb. 16-szor hamarabb érkezik meg. Az első átkódolt HLS-szegmens kb. 1
  másodperccel korábban, és egyenletesebben (p95 2,0 s a 4,2 s-mal szemben).

### Ahol a Kinora rosszabb

- **Üresjárati CPU:** átlagosan 0,4%, a Jellyfiné 0,1%. A Kinora alapterhelése kb. 0,08%, de nagyjából 30 másodpercenként
  egy-egy 4–5%-os kiugrás jelentkezik; a Jellyfiné kb. 0,02%. Abszolút értékben kicsi, de mérhető különbség; a forrását
  még nem kerestük meg ([#13](https://github.com/ScotyLucas/Kinora/issues/13)).
- **Átlagos CPU terhelés alatt:** 394% a Jellyfin 353%-ával szemben, a 4 mag szinte teljes kihasználása. Mivel közben
  9-szer annyi kérést szolgált ki, ez kérésenként kevesebb CPU-t jelent.

## Mit mér pontosan

- **Szkennelés:** a könyvtárak hozzáadásától (Kinora) vagy a `POST /Library/Refresh`-től (Jellyfin) addig, amíg a szerver
  szerint véget ért (Kinora: `/api/admin/scan`, Jellyfin: a „Scan Media Library” ütemezett feladat). Utána ugyanez még
  egyszer, változás nélkül.
- **Metaadat:** egyik szerver sem tölt le semmit az internetről. A Kinorában nincs TMDB-kulcs. A Jellyfin-könyvtárakban a
  metaadat-letöltők ki vannak kapcsolva, a helyi képforrások (beágyazott kép, Screen Grabber) be, mert a Kinora is készít
  előnézeti képkockát. A fejezet- és trickplay-képek szkennelés közben nem készülnek (ez a Jellyfin alapbeállítása).
  Minden más alapbeállítás. A Kinora intro-felismerése a szkennelés után, a háttérben fut, a szkennelési időben nincs
  benne.
- **API:** amit a webes kliens kér:
  - **Kinora:** főoldal `GET /api/home`; könyvtár `GET /api/items?library=…&sort=title&limit=60`; keresés
    `GET /api/search?group=1&q=river`.
  - **Jellyfin:** főoldal a webes kliens öt párhuzamos kérése (`/UserViews`, `/UserItems/Resume`, két
    `/Items/Latest`, `/Shows/NextUp`), az eredmény az összes megérkezéséig eltelt idő; könyvtár
    `GET /Items?ParentId=…&SortBy=SortName&Limit=60`; keresés egyetlen `GET /Items?SearchTerm=river&Limit=60` (a Jellyfin
    webes kliense ennél több kérést is küldhet).
  - A „river” szó a címek 1/25-ében szerepel.
- **Terhelés:** 16 kliens körbe-körbe kéri a három forgatókönyvet 60 másodpercig; a „művelet” egy forgatókönyv.
- **Lejátszás:** a lejátszási kéréstől (Kinora: `POST /api/items/{id}/playback`, Jellyfin: `POST /Items/{id}/PlaybackInfo`)
  az első 1 MiB megérkezéséig (direct play), illetve az első HLS-szegmens teljes letöltéséig (HEVC/AC-3 → H.264/AAC
  átkódolás, szoftveres libx264, hardveres gyorsítás nélkül).
- **CPU és RAM:** a host `docker stats` mintái kb. 2 másodpercenként; a RAM a konténer memóriahasználata a lapgyorsítótár
  nélkül.

## Korlátok

- Egy laptop, egy futás. A szkennelési ismétlések közötti szórás kb. 15–20% (hőmérséklet, a hibrid CPU ütemezése);
  a kisebb különbségeket ezért nem érdemes túlértékelni.
- A tesztkönyvtár rövid, egyforma klipekből áll, metaadat nélkül. Valódi filmeknél az ffprobe, a képkocka-előnézet és
  az átkódolás aránya más lehet.
- A mérés a két szerver saját API-ját használja. A „főoldal” a két kliensben nem pontosan ugyanazt jelenti; ezért van
  leírva, mit kér egyik és másik.

## A mérés közben talált és javított hibák

A benchmark két teljesítményhibát talált a Kinorában. A fenti számok már a javítás után készültek
(commit `6990e86`):

- **Tétlen SQLite-kapcsolatok:** a `database/sql` alapból csak 2 tétlen kapcsolatot tart meg. Párhuzamos
  kéréseknél a kapcsolatok folyton bezárultak és hidegen nyíltak újra. Most 16 marad nyitva.
- **Az SQLite globális memóriastatisztika-zára:** az alapértelmezett fordítás (`SQLITE_DEFAULT_MEMSTATUS=1`) minden
  memóriafoglalásnál egy közös zárat használ, ez sorba állította a párhuzamos lekérdezéseket. Nélküle a keresés 4 CPU-n,
  16 klienssel 529-ről 1412 kérés/s-ra nőtt (a 3 forgatókönyves keverék itt 162 művelet/s, mert a főoldal jóval
  drágább).

Módszertani tanulság: CPU-kvótával (`cpus: 4`) ezen a hibrid laptop-CPU-n mindkét szerver áteresztőképessége
összeomlott párhuzamos terhelésnél. Ezért a mérés rögzített CPU-készletet (`cpuset`) használ.

---

## English summary

Measured on 2026-11-23 on one laptop (Intel Core i5-1235U, Windows 11 + Docker Desktop/WSL2). Both servers ran in turn on
the same 4 pinned CPUs with a 4 GB limit, on the same generated library of 10 002 videos (5000 films, 500 series × 10
episodes, short clips). Jellyfin 12.4.0; neither server fetched internet metadata. Reproduce with `scripts/bench/`; raw
data in `scripts/bench/results/20261123-1908/`.

| | Kinora | Jellyfin |
|---|---|---|
| First scan (median of 3) | **565.5 s** | 845.5 s |
| Rescan, nothing changed | **0.3 s** | 21.8 s |
| Idle RAM / CPU | **103 MiB** / 0.4% | 472 MiB / **0.1%** |
| RAM under load (peak) | **167 MiB** | 488 MiB |
| Home / library page / search, p50 | **29.5 / 7.2 / 9.6 ms** | 127.4 / 59.7 / 86.0 ms |
| 16 concurrent clients | **162 ops/s**, 0 errors | 17 ops/s, 0 errors |
| Direct play, first MiB (p50) | **4.7 ms** | 74.8 ms |
| Transcode, first HLS segment (p50 / p95) | **1995 / 2000 ms** | 3018 / 4156 ms |

Where Kinora is worse: idle CPU (0.4% vs 0.1%; Kinora idles at about 0.08% with a 4–5% spike roughly every 30 seconds; the
cause is not known yet, [#13](https://github.com/ScotyLucas/Kinora/issues/13)) and average CPU under load (394% vs 353%, while serving nine times as many requests). Limits: one
machine, one run, short identical clips; see the Hungarian sections above for exactly what each number measures. The
benchmark also found two Kinora performance bugs (idle SQLite connections, SQLite's global memory-statistics lock), fixed
before these numbers were taken.
