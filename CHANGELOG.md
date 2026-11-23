# Változásnapló / Changelog

A kiadások részletes jegyzete: [docs/releases/](docs/releases/). Ez a fájl a még ki nem adott változásokat gyűjti.
/ Detailed notes of each release: [docs/releases/](docs/releases/). This file collects the changes not released yet.

## Kiadatlan / Unreleased

### Magyar

**Új**
- **Váltás Jellyfinről:** Admin → Import → Jellyfin. Felhasználók (jelszó nélkül, egyszer használható jelszóbeállító
  linkkel), megnézett állapot, folytatási pozíció, kedvencek és utolsó lejátszás. Párosítás fájlútvonal (útvonal-
  leképezéssel), TMDB/IMDb/TVDB azonosító, epizódnál évad és epizódszám alapján; előnézet, jóváhagyás, idempotens
  újrafuttatás, letölthető jelentés.
- **Váltás Sonarrról, Radarrról, Prowlarrról:** figyelt sorozatok és filmek, minőségprofilok, indexerek,
  letöltőkliensek és gyökérmappák; ugyanígy előnézettel és jelentéssel.
- **Jellyfin-kompatibilis API** (alapból kikapcsolva, külön port): Jellyfin-kliensek csatlakozhatnak, a Kinora
  jogosultságaival; Quick Connect profil- és PIN-választással. Kodi + JellyCon-nal kipróbálva.
- **Kezdő beállítási varázsló:** médiamappák, TMDB-kulcs, hardveres gyorsítás tesztje, letöltőkliens és indexer,
  távoli elérés, összefoglaló; minden lépés kihagyható és később pótolható (HU/EN/DE).
- **Offline letöltés:** az Android-app a rendszer letöltéskezelőjével tölt le, és az indítóból szerver nélkül is
  lejátszik; az iOS-app szerver nélkül indulva a letöltéseket mutatja. A letöltések profilhoz kötöttek (gyerekprofil
  más profil letöltését nem látja, offline sem); az offline megnézett pozíció a következő csatlakozáskor a szerverre
  kerül.
- **Benchmark:** `scripts/bench/` – Kinora és Jellyfin mérése egymás mellett Docker Compose-zal.

**Javítva**
- CSRF-védelem a sütis munkamenetekhez; a belső hibák részletei nem jutnak ki; jogosultsági rések bezárva.
- Közös letöltési feladatok, korlátos sorok és átkódolási limit; a háttérfolyamatok túlélik a pánikot.
- Újabb verzió által írt adatbázist a szerver nem nyit meg.
- A fuzz-tesztek által talált hibák a fájlnév-, release- és Cardigann-elemzőkben.
- Jellyfin-kompatibilis API: WebSocket, `/Years` és `/Tags`, típusszűrés a „Latest” listán, feliratmezők, token nélküli
  stream-URL a lejátszási munkamenettel (a Kodi-teszt során találva).
- Teljesítmény: a tétlen SQLite-kapcsolatok nyitva maradnak, és az SQLite a globális memóriastatisztika-zár nélkül
  fordul; párhuzamos kéréseknél ez többszörös áteresztőképességet adott (a mérés: docs/BENCHMARKS.md).

**Fejlesztés**
- CI: `staticcheck`, `govulncheck`, fuzz-tesztek, az appok tesztjei, problémás médiafájlok generátora és integrációs
  tesztjei; CI-jelvény a README-ben.
- Dokumentáció: ARCHITECTURE, AUDIT, IMPORT, JELLYFIN_COMPAT, OFFLINE, BENCHMARKS, CONTRIBUTING, SECURITY; issue- és
  PR-sablonok; a README összehasonlító táblázata átírva.

### English

**New**
- **Switching from Jellyfin:** Admin → Import → Jellyfin. Users (without passwords, with a one-time set-password link),
  watched state, resume positions, favourites and last played. Matching by file path (with path mapping), TMDB/IMDb/TVDB
  id, and season and episode number; preview, approval, idempotent re-runs, downloadable report.
- **Switching from Sonarr, Radarr, Prowlarr:** monitored series and films, quality profiles, indexers, download clients
  and root folders; with the same preview and report.
- **Jellyfin-compatible API** (off by default, separate port): Jellyfin clients can connect, with Kinora's permissions;
  Quick Connect with profile and PIN choice. Tried with Kodi + JellyCon.
- **First-run wizard:** media folders, TMDB key, hardware acceleration test, download client and indexer, remote
  access, summary; every step can be skipped and done later (HU/EN/DE).
- **Offline downloads:** the Android app downloads with the system's download manager and plays from the launcher
  without the server; the iOS app shows the downloads when it starts without the server. Downloads belong to profiles
  (a kids profile never sees another profile's, offline either); positions watched offline reach the server on the
  next connection.
- **Benchmark:** `scripts/bench/` – Kinora and Jellyfin measured side by side with Docker Compose.

**Fixed**
- CSRF protection for cookie sessions; internal error details no longer leak; permission gaps closed.
- Shared download jobs, bounded queues and a transcode limit; background loops survive panics.
- The server refuses a database written by a newer version.
- Bugs found by fuzzing in the file name, release and Cardigann parsers.
- Jellyfin-compatible API: WebSocket, `/Years` and `/Tags`, type filter on the Latest list, subtitle fields, stream URLs
  without a token via the play session (found while testing with Kodi).
- Performance: idle SQLite connections stay open, and SQLite is built without its global memory-statistics lock;
  under concurrent requests this gave several times the throughput (measured: docs/BENCHMARKS.md).

**Development**
- CI: `staticcheck`, `govulncheck`, fuzz tests, app tests, a problem-media generator with integration tests; CI badge in
  the README.
- Documentation: ARCHITECTURE, AUDIT, IMPORT, JELLYFIN_COMPAT, OFFLINE, BENCHMARKS, CONTRIBUTING, SECURITY; issue and PR
  templates; the README comparison table rewritten.
