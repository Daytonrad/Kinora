# Kinora „Beszerzés” modul – teljes terv

*2026-11-19 · cél: a Jellyseerr, Radarr, Sonarr és Prowlarr kiváltása beépített Kinora-funkcióval. Semmit nem kell külön telepíteni: a szerverrel együtt jön, alapból ki van kapcsolva, az admin kapcsolja be.*

---

## 0. Állapot

- **M1 kész** (2026-11-19): `internal/acquire` (kiadásnév-értelmező, minőségprofil, Torznab, .torrent/magnet + infohash, qBittorrent 4.x/5.x, import hardlinkkel), `internal/store/acquire.go` (6. migráció), `internal/server/acquire.go` (admin API), Admin → Beszerzés (Filmek keresése, Letöltési sor, Források és kliens), hu/en/de.
- Kipróbálva valódi **qBittorrent 5.3**-mal: indexer- és kliensteszt, keresés, pontozás (a CAM elutasítva), letöltés, a qBittorrent ellenőrzi a meglévő adatot, import hardlinkkel (azonos inode), beolvasás, a film megjelenik a könyvtárban.
- A TMDB címe a `KINORA_TMDB_API` változóval átirányítható (proxy, tükör, tesztek).

## 1. Döntések (a beszélgetés alapján)

| Kérdés | Döntés |
|---|---|
| Külön appok vagy beépítve? | **Beépítve** a Kinora szerverbe (Go), nincs Radarr/Sonarr/Prowlarr/Jellyseerr. |
| Ki kérhet? | **Bárki, akinek van fiókja**, a gyerekprofil kivételével. Az admin jóváhagyást és kvótát állíthat be (M2). |
| Letöltőkliens | **qBittorrent** elsőként; közös interfész mögött később Transmission, Deluge, Usenet (SABnzbd/NZBGet). A Kinora szerveroldalon vezérli az API-jukon át. |
| Hol fut? | Minden a szerveren. A kliensek (web, appok) csak a Kinora API-ját látják; indexer-kulcs és kliens-jelszó soha nem jut ki. |
| Alapállapot | **Kikapcsolva** (`acq_enabled=0`), beépített trackerlista és indexer-leíró nélkül: a Kinora semleges eszköz marad. |

---

## 2. Mit vált ki és mivel

| Kiváltott app | Kinora-rész | Mérföldkő |
|---|---|---|
| Prowlarr | Indexerek kezelése: szabványos **Torznab** (torrent) és később **Newznab** (Usenet) API; tesztelés, prioritás, állapot | M1 (Torznab), M4 (Newznab) |
| Prowlarr leírói | **Beépített indexer-motor:** oldal-leírók (Cardigann-formátumú YAML) futtatása, így a Torznab-ot nem beszélő oldalak is működnek proxy nélkül | M5 |
| Radarr | Filmkeresés, kiadásnév-értelmezés, minőségprofil, pontozás, letöltés, importálás, átnevezés | M1 (kézi), M2 (automatikus) |
| Sonarr | Sorozatfigyelés, RSS, hiányzó részek, évadcsomagok | M3 |
| Jellyseerr | Felfedezés, „Kérem” gomb, jóváhagyás, kvóta, „Kéréseim”, értesítések | M2 |

> **Fontos korlát az M5-ig:** a legtöbb tracker nem beszél Torznab-ot. Az M1–M4 alatt ezekhez egy indexer-proxy (Jackett vagy Prowlarr) **egyetlen Torznab-forrásként** használható, vagy olyan tracker, amelynek van Torznab-kompatibilis API-ja. A teljes „semmit nem kell telepíteni” állapot az M5-tel érkezik.

---

## 3. Architektúra

```
internal/acquire/            új Go csomag (UI nélkül, tesztelhető)
  release.go                 kiadásnév-értelmező: cím, év, évad/rész, felbontás, forrás, kodek, HDR, hang, nyelv (HUN), csoport, PROPER/REPACK
  quality.go                 minőségprofil + pontozás (miért ennyi pont: emberi nyelvű indoklás)
  torznab.go                 Torznab-kliens (caps, keresés imdbid/tmdbid/cím alapján, RSS + torznab:attr értelmezése)
  torrent.go                 .torrent letöltése, infohash számítása (bencode), magnet-értelmezés
  client.go                  DownloadClient interfész
  qbittorrent.go             qBittorrent WebUI API v2 (login, kategória, hozzáadás, állapot, törlés)
  importer.go                kész letöltés → könyvtár: fő videófájl kiválasztása, hardlink/másolás, feliratok, elnevezés
  manager.go                 keresés → pontozás → letöltés → állapotkövetés (poller) → import → beolvasás
internal/store/acquire.go    táblák és lekérdezések
internal/server/acquire.go   HTTP API
web/static/app.js            Admin → Beszerzés oldalak (M1), Felfedezés / Kéréseim (M2)
```

**Folyamat egy filmnél:**
TMDB-találat → indexer-keresés (IMDb-azonosító, különben cím + év) → értelmezés és pontozás a profil szerint → a legjobb (vagy kézzel választott) kiadás → a Kinora letölti a .torrentet vagy a magnetet, kiszámolja az infohash-t → qBittorrent (`kinora` kategória, `kinora-<id>` címke) → a poller 20 mp-enként frissíti a % / sebesség / ETA értékeket → kész: **hardlink** a könyvtárba (a torrent tovább seedel, nincs dupla hely), különben másolás → `Cím (Év) {imdb-tt…}/Cím (Év) - 1080p.mkv` → az adott könyvtár beolvasása → a film megjelenik.

**Útvonal-leképezés:** a qBittorrent a saját fájlrendszerében jelenti a letöltés helyét (pl. `/downloads/kinora/…`), a Kinora máshol láthatja (pl. `/data/downloads/kinora/…`). Kliensenként egy „távoli útvonal → helyi útvonal” pár állítható. Hardlinkhez a letöltési és a könyvtármappának **ugyanazon a fájlrendszeren** kell lennie (Dockerben közös `/data` csatolás).

---

## 4. Adatmodell

```sql
acq_indexers(id, name, kind 'torznab', url, api_key, categories, enabled, priority,
             last_ok_at, last_error, created_at)
acq_clients(id, name, kind 'qbittorrent', url, username, password, category, save_path,
            path_remote, path_local, enabled, created_at)
acq_grabs(id, kind 'movie', tmdb_id, imdb_id, title, year, library_id,
          release_title, indexer_id, client_id, info_hash, size, quality, score,
          status 'downloading|importing|done|failed|removed', progress, eta, speed, error,
          target_path, requested_by, created_at, updated_at)
-- M2: acq_requests(id, user_id, profile_id, kind, tmdb_id, season_set, status 'pending|approved|denied|available', …)
-- M3: acq_series(series_tmdb_id, monitor 'all|future|none', profile_id, library_id, …), acq_wanted(episode…)
-- M2+: acq_blocklist(info_hash, reason, created_at), acq_profiles(id, name, json)
```

**Beállítások (`settings` tábla):** `acq_enabled` (0/1), `acq_profile` (JSON: engedett felbontások, min./max. méret GB-ban, min. seeder, magyar előny, tiltott források: CAM/TS/TC/SCREENER), `acq_import_mode` (`hardlink` | `copy`).

---

## 5. API

**M1 – csak admin** (`/api/admin/acquire/…`):

| Végpont | Leírás |
|---|---|
| `GET/PUT settings` | bekapcsolás, profil, importmód |
| `GET/POST indexers`, `PATCH/DELETE indexers/{id}`, `POST indexers/test` | indexerek kezelése és tesztelése (mentés előtt is) |
| `GET/POST clients`, `PATCH/DELETE clients/{id}`, `POST clients/test` | letöltőkliens kezelése és tesztelése |
| `GET lookup?q=` | TMDB filmkeresés, „megvan a könyvtárban” jelzéssel |
| `GET releases?tmdb_id=` | kiadások az összes indexerről, pontozva, indoklással, rendezve |
| `POST grab` | `{tmdb_id, library_id, release}` → letöltés indul |
| `GET queue` | letöltések állapota (élő %) |
| `DELETE grabs/{id}?files=0/1` | megszakítás / eltávolítás a kliensből |
| `POST grabs/{id}/retry` | sikertelen import újrapróbálása |

**M2 – felhasználói:** `GET /api/discover`, `POST /api/requests`, `GET /api/requests` (saját), admin: `PATCH /api/admin/requests/{id}` (jóváhagy/elutasít).

A titkos mezők (API-kulcs, jelszó) a meglévő mintát követik: maszkolva mennek ki, a maszkolt érték visszaküldése nem írja felül őket.

---

## 6. Felület

**M1 – Admin → új „Beszerzés” csoport:**
- **Filmek keresése:** keresőmező → TMDB-találatok poszterrel. A már meglévők „Megvan” jelzést kapnak. Egy filmre kattintva megjelennek a kiadások táblázata: név, felbontás/forrás/kodek jelvények, méret, seederek, indexer, pontszám, indoklás („1080p BluRay, magyar hang +, 12 GB”). Választható célkönyvtár (filmkönyvtárak), „Legjobb letöltése” és soronként „Letöltés”.
- **Letöltések:** sor élő %-kal, sebességgel, ETA-val, állapottal; megszakítás, újrapróbálás.
- **Beállítások:** be-/kikapcsolás; indexerek listája (hozzáadás, teszt, ki-/bekapcsolás, törlés); qBittorrent (cím, felhasználó, jelszó, kategória, mentési mappa, útvonal-leképezés, teszt); minőségprofil; importmód.

**M2 – mindenkinek:** „Felfedezés” menüpont (keresés, népszerű, hamarosan érkező, ajánlott), „Kérem” gomb, „Kéréseim”, jóváhagyási lista az adminnak, értesítés, ha kész. Az Android/Samsung/LG/Titan appokban automatikusan megjelenik (webkliens); iOS/tvOS natív felület az M4-ben.

---

## 7. Mérföldkövek

| | Tartalom | Kész, ha… |
|---|---|---|
| **M1 – filmek, admin kézzel** | Torznab, qBittorrent, kiadásnév-értelmező, egy minőségprofil, pontozás, admin keresőoldal, letöltési sor, import hardlinkkel, azonnali beolvasás, magyar/angol/német felület | admin a Kinorában rákeres egy filmre, letölti, és az magától megjelenik a könyvtárban |
| **M2 – kérések** | Felfedezés oldal, „Kérem” mindenkinek (gyerek kivételével), automatikus jóváhagyás vagy admin jóváhagyás, kvóta, automatikus keresés és letöltés, hibánál feketelista + következő kiadás, RSS-figyelés a még meg nem jelent filmekre, értesítések | egy családtag kér egy filmet, és az emberi beavatkozás nélkül megérkezik |
| **M3 – sorozatok** | Figyelt sorozatok (TMDB-adások), RSS-szinkron, hiányzó részek keresése, évadcsomagok, sorozat-elnevezés (`Sorozat/Season 01/…S01E03`), évadonkénti kérés | új rész megjelenése után magától letöltődik |
| **M4 – kényelem** | Csere jobb minőségre (cutoff), több minőségprofil, Transmission/Deluge, Newznab + SABnzbd/NZBGet, statisztika, iOS/tvOS kérés-felület, értesítés push/Live Activity | |
| **M5 – beépített indexer-motor** | Cardigann-formátumú oldal-leírók futtatása (bejelentkezés, keresés, eredmény-kinyerés), így Jackett/Prowlarr sem kell; a leírókat az admin tölti fel, a Kinora nem szállít trackerlistát | egyetlen Kinora-telepítéssel működik a teljes lánc |

---

## 8. Biztonság és üzemeltetés

- Minden beszerzés-végpont admin-jogosultságú (M1); a kéréseknél a kérő csak a saját kéréseit látja (M2).
- A Kinora csak az admin által megadott címeket hívja (indexer, kliens). Ezek a szerver belső hálózatát is elérhetik, ezért csak admin adhatja meg őket.
- A letöltött .torrent mérete korlátozott (pl. 10 MB), az időkorlátok szigorúak, és a magnet-, illetve http(s)-linkeken kívül mást nem követ.
- Az import csak a beállított könyvtárgyökér alá ír, a fájlneveket megtisztítja, és nem követ szimbolikus linket kifelé.
- Docker-javaslat: közös `/data` (benne `downloads/` és `media/`) a Kinorának és a qBittorrentnek, így működik a hardlink.

---

## 9. M1 tesztelés

- Go egységtesztek: kiadásnév-értelmező (valós névminták), pontozás, Torznab XML-fixture, bencode/infohash, magnet, qBittorrent-kliens hamis szerverrel, import (hardlink, másolás, fő fájl kiválasztása, sample kihagyása, felirat), teljes manager-folyamat hamis indexerrel és klienssel.
- Valódi qBittorrent konténer: belépés, kategória, **szüneteltetett** hozzáadás egy szabad licencű torrenttel (pl. Big Buck Bunny), állapot, törlés.
- Webes e2e: admin oldalak megjelennek, keresés → kiadáslista → letöltés a hamis szolgáltatásokkal.
