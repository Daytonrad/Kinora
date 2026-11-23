# Közreműködés

**Magyar** · [English](#contributing-english)

Köszönjük, hogy segítenél! Hibajelentés, ötlet, fordítás, dokumentáció és kód egyaránt jól jön. Sérülékenységet ne
nyilvános issue-ban jelents, hanem a [SECURITY.md](SECURITY.md) szerint.

## Fejlesztői környezet

- **Szerver:** Go 1.27 és cgo (az SQLite miatt C-fordító kell), valamint `ffmpeg`/`ffprobe` a `PATH`-on.
  Helyi Go nélkül minden Go-parancs futtatható Dockerben: `scripts/go.sh go test ./...` (a `kinora-dev` image-et
  magától felépíti).
- **Webes kliens:** `web/static/` – sima JavaScript, build-lépés nincs; a szerver beágyazza.
- **Kliensappok:** `apps/` (Node 22+, `cd apps && npm install`), az iOS-app macOS-en Xcode-dal; részletek:
  [apps/README.md](apps/README.md).
- **Indítás:** `go run ./cmd/kinora --data ./data`, majd `http://localhost:8096`. Docker Compose-zal:
  `docker compose -f docker-compose.yml -f docker-compose.dev.yml up --build`.
- **Felépítés:** [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md).

## Tesztek

| Parancs | Mit ellenőriz |
|---|---|
| `scripts/mkmedia.sh testmedia` | egyszer: a tesztkönyvtár (rövid, generált videók) a teszteknek |
| `go vet ./...` és `go test -race ./...` | egység- és integrációs tesztek (`httptest`), versenyhelyzet-ellenőrzéssel |
| `scripts/smoke.sh` | Docker-image build, konténer indítása, a fő folyamatok végigjárása curl-lel; `SMOKE OK`-val kell végződnie |
| `scripts/fuzz.sh` | Go fuzz-tesztek (fájlnév-, NFO-, release- és Cardigann-elemzők); a talált hibák bemenete regressziós teszt lesz |
| `scripts/gen-testmedia.sh` | problémás médiafájlok (HDR10, PGS, sok hangsáv, VFR, sérült fájl, Unicode-útvonal) az integrációs tesztekhez |
| `cd apps && npm test` | a kliensappok közös egységtesztjei |
| `BASE=… node scripts/e2e.mjs` | böngészős végigjárás (Playwright, a rendszer Edge/Chrome-ja), képernyőképekkel |
| `scripts/bench/bench.sh` | Kinora és Jellyfin mérése egymás mellett ([docs/BENCHMARKS.md](docs/BENCHMARKS.md)) |

A CI (`.github/workflows/ci.yml`) minden pushra és PR-re lefuttatja: `go vet`, `go test -race`, `staticcheck`,
`govulncheck`, az appok tesztjei és a Docker-image build. Az appok buildjei (Android, iOS, TV) az `apps.yml`-ben vannak.

## Szabályok a kódhoz

- Minden új funkcióhoz teszt kell (egységteszt, és ahol lehet, `httptest`-es integrációs teszt).
- Minden új végpont a meglévő hitelesítésen és jogosultságkezelésen megy át; a gyerekprofil és a könyvtár-láthatóság
  mindenhol érvényes.
- SQL csak paraméterezve, string-összefűzés nélkül; fájlrendszer-útvonalaknál védekezés az útvonal-bejárás ellen.
- Új függőséget csak indokolt esetben veszünk fel – a kevés függőség a Kinora egyik előnye. Kérdezz előtte egy issue-ban.
- Felhasználónak szóló szöveg mindig a nyelvi fájlokba kerül (`web/static/i18n/hu.js`, `en.js`, `de.js`), mindhárom
  nyelven; a `web/i18n_test.go` ellenőrzi.
- A dokumentációban csak igaz, ellenőrizhető állítás szerepeljen. Más szoftverről (például a Jellyfinről) ne írjunk
  olyat, hogy „nincs benne X”, ha nem vagyunk benne biztosak.

## Commit-üzenetek

Rövid, angol nyelvű tárgysor, előtaggal és nem kötelező hatókörrel:
`feat(scanner): …`, `fix(server): …`, `perf(store): …`, `docs: …`, `test: …`, `ci: …`, `chore: …`, `style: …`.
A törzsben az írja le a miértet, ami a diffből nem derül ki.

## Pull requestek

1. Ágazz le a `main`-ről (a `main`-re közvetlenül senki nem pushol).
2. Kis, egy témájú PR-ek; ha nagyobb változtatásba kezdenél (új API, új függőség), előbb nyiss egy issue-t.
3. A PR-sablon kérdéseire válaszolj: mit változtat, hogyan tesztelted, kell-e dokumentáció.
4. A CI-nak zöldnek kell lennie. A tulajdonos nézi át és olvasztja be; kiadást csak ő készít.

## AI-használat a projektben

A Kinora kódjának és dokumentációjának nagy részét a tulajdonos irányításával egy AI-asszisztens (Anthropic Claude,
Claude Code) írta. Ezek a commitok `Co-Authored-By: Claude …` sort viselnek; 2026 novemberében a 190 commitból 175
ilyen volt. A feladatokat, a döntéseket és a jóváhagyást a tulajdonos adja. Minden változásnak át kell mennie a
tesztcsomagon és a CI-n, és a fontosabb részeket (biztonság, jogosultságok, adatmigráció) külön tesztek fedik le.

AI-eszközt te is használhatsz a közreműködéshez. Ilyenkor:

- jelöld a PR-ben (és ha akarod, `Co-Authored-By` sorral a commitban);
- csak olyan kódot küldj, amit érted, és amit lefuttattál a tesztekkel;
- ne másolj be más projekt kódját (a Jellyfin például GPL-licencű: a Jellyfin-kompatibilis API csak a nyilvános
  API viselkedését valósítja meg).

---

## Contributing (English)

Thanks for helping! Bug reports, ideas, translations, documentation and code are all welcome. Please report security
issues privately as described in [SECURITY.md](SECURITY.md), not in a public issue.

**Setup.** Server: Go 1.27 with cgo (SQLite needs a C compiler) and `ffmpeg`/`ffprobe` on the `PATH`; without a local
Go toolchain, run any Go command in Docker: `scripts/go.sh go test ./...`. Web client: plain JavaScript in
`web/static/`, no build step. Apps: `apps/` (Node 22+, `npm install`), iOS on macOS with Xcode — see
[apps/README.md](apps/README.md). Architecture: [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md).

**Tests.** Generate the test library once with `scripts/mkmedia.sh testmedia`, then `go vet ./...`,
`go test -race ./...` and `scripts/smoke.sh` (must end with `SMOKE OK`). Also available: `scripts/fuzz.sh`,
`scripts/gen-testmedia.sh`, `cd apps && npm test`, `node scripts/e2e.mjs` and `scripts/bench/bench.sh`. CI runs vet,
race tests, staticcheck, govulncheck, the app tests and the Docker build on every push and pull request.

**Rules.** Every new feature comes with tests. Every new endpoint goes through the existing authentication and
permission checks; kids profiles and library visibility apply everywhere. Parameterised SQL only; guard file paths
against traversal. New dependencies only when justified — ask in an issue first. User-facing text goes into the
language files in Hungarian, English and German. Documentation states only what is true and verifiable.

**Commits.** A short English subject with a prefix and optional scope (`feat(scanner): …`, `fix: …`, `docs: …`,
`test: …`, `ci: …`, `perf: …`, `chore: …`); the body explains why.

**Pull requests.** Branch from `main` (nobody pushes to `main` directly), keep PRs small and focused, open an issue
before larger changes (new APIs, new dependencies), fill in the PR template, and keep CI green. The owner reviews,
merges and makes releases.

**AI use.** Most of Kinora's code and documentation was written by an AI assistant (Anthropic Claude, Claude Code)
under the owner's direction; those commits carry a `Co-Authored-By: Claude …` line (175 of 190 commits as of November
2026). The owner sets the tasks, makes the decisions and approves the changes; every change has to pass the test suite
and CI. You may use AI tools too: say so in the PR, send only code you understand and have tested, and don't paste code
from other projects (Jellyfin, for example, is GPL-licensed; Kinora's Jellyfin-compatible API implements only the
public API's behaviour).
