# Product

<!-- impeccable:product-schema 1 -->

## Platform

web

## Users

- **Családtagok a nappaliban**, este, TV-böngészőn vagy TV-re kötött gépen, távirányítóval / D-paddel, kb. 3 méterről. Feladat: gyorsan megtalálni, mit nézzenek, folytatni a megkezdettet, elindítani a következő részt.
- **Ugyanők telefonon**, útközben vagy ágyban, egy kézzel: folytatás, böngészés, letöltés offline-ra.
- **Laptopon / asztali gépen**: böngészés, gyűjtemények és címkék szerkesztése, admin feladatok (könyvtárak, felhasználók, átkódolás, metaadat).
- **Meghívott barátok és rokonok távolról**, nem technikai felhasználók: meghívólinkkel regisztrálnak, watch partyhoz csatlakoznak kóddal.
- Egy fiók egy háztartás; fiókonként legfeljebb 8 profil, köztük PIN-es és gyerekprofil.

## Product Purpose

Saját gépen futó médiaszerver filmekhez és sorozatokhoz (Jellyfin/Plex-alternatíva), magyar felülettel. Siker: a háztartás tagjai a saját eszközükön, pár lépésben elindítják, amit nézni akarnak, és a szerver „csak működik” (szkennelés, átkódolás, megnézett állapot) anélkül, hogy az admin naponta foglalkozna vele.

## Positioning

Azokat a Jellyfin-panaszokat oldja meg, amelyek a mindennapi használatot rontják, és ezt a felületen is megmutatja: a lejátszási döntés indokai láthatók („Miért így megy?”), az encoder-önteszt valóban kódol, az áthelyezett fájl megtartja a megnézett állapotot, a hálózati megosztás kiesése nem töröl semmit, van elrejtés megnézettnek jelölés nélkül, helyi (nem felhős) ajánló, smart gyűjtemények és szerver-órás watch party. A konkurens nem tudja ugyanezt az átláthatóságot ígérni: a Kinora megmondja, miért kódol át, és mit választott.

## Operating Context

- Elsődleges telepítés Docker (`docker compose up -d`), a média csak olvashatóan `/media` alatt; Unraid/TrueNAS/Synology NAS-ok gyakoriak.
- A könyvtárak gyakran **kép nélküliek**: self-hosted gyűjteményben a poszter/backdrop hiányozhat (NFO nélkül, TMDB-kulcs nélkül). A felületnek kép nélkül is rendben kell kinéznie; a Kinora videókockából készít thumbnailt.
- Esti használat, sötét szobában, nagy képernyőn: a világos felületek vakítanak.
- A felhasználók eszközei vegyesek: Windows, Android, iOS, TV-böngészők (régebbi WebView-k is), ezért a betűtípusok nem lehetnek rendszerfüggők.
- A watch party WebSocketen fut; reverse proxy / Tailscale / Cloudflare Tunnel mögött is.

## Capabilities and Constraints

- Vanilla JS SPA build-lépés nélkül (`web/static/index.html`, `app.js`, `style.css`), hash-routing; vendorolt libek: hls.js, qrcode-generator. CDN nem használható (offline/LAN-only szerverek), a fontokat is a szerver szolgálja ki (`web/static/fonts`).
- Nézetek: setup-varázsló, bejelentkezés, meghívós regisztráció, profilválasztó, kezdőlap (hero + sorok), könyvtár (szűrők, végtelen görgetés), keresés, részletes oldal (film / sorozat / évad / epizód), lejátszó (saját kontrollok, feliratok, minőség, why-panel, következő rész, watch party chat), gyűjtemények + smart-szerkesztő élő előnézettel, letöltések, fiók, admin (irányítópult, könyvtárak, felhasználók, meghívók, átkódolás, metaadat, csatlakozás, beállítások).
- A funkciók és az API nem változnak a redesignban; a böngészős E2E (`scripts/e2e.mjs`) és a `verify` skill zöld kell maradjon.
- Minden UI-szöveg az `app.js` elején lévő `T` objektumban van; **később angol fordítás készül**, ezért szöveg nem kerülhet CSS-be vagy képbe.
- Terminológia (kötött): Folytatás, Következő rész, Neked ajánljuk, Mert megnézted, Rejtett kincsek, Elrejtés, Watch party, Smart gyűjtemény, Könyvtár, Profil, Gyerekprofil.
- Korhatár-szűrés minden végponton; gyerekprofil nem szerkeszthet.

## Brand Commitments

- Név: **Kinora** (az 1890-es évek kézi mozgókép-nézegetőjéről). Magyar UI.
- **A kezdőlap hero-kompozíciója marad**: nagy cím, indok-sor („Mert szereted…”), két művelet (Lejátszás/Folytatás + Részletek) a backdrop felett.
- A tulajdonos által rögzített vizuális megkötések (a redesign briefje, nem bővítendő): esti mozi, meleg és nyugodt, nem neon; sötét alap, de nem tiszta fekete és nem kék-lila gradiens; egyetlen meleg akcentszín (borostyán / vetítő-sárga); poszterek és backdropok a főszereplők, a UI hátralép; karakteres címfont + jól olvasható szövegfont, önállóan kiszolgálva; nincs kártya a kártyában, nincs minden-dobozban, nincs gradiens gomb, glow, emoji ikonként, felesleges árnyék; egységes design tokenek (szín, térköz-, betűméret-skála, lekerekítés, animációs időzítés), hardkódolt érték nélkül; TV-n és telefonon is működjön: nagy fókuszjelzés, 44 px-es minimum érintési felület.
- Két vizuális regiszter elfogadva: „előcsarnok” (böngészés: kép vezet) és „vetítőfülke” (admin, szerkesztők: csendes, pontos), közös tokenekkel.

## Evidence on Hand

- Generált tesztkönyvtár (`scripts/mkmedia.sh`): szintetikus színcsíkos videók, NFO-k, CP1250-es felirat – **nincs valódi poszter vagy backdrop**; a képernyőképeken a placeholder látszik.
- Képernyőképek a jelenlegi felületről: `docs/screenshots/` és a kritika-snapshot (`.impeccable/critique/`).
- README magyarul, „Miért jobb, mint a Jellyfin” táblázattal.
- Nincs: logó, ikonkészlet, fotók, felhasználói visszajelzés, ajánlások. Ezeket nem szabad kitalálni.

## Product Principles

1. **A tartalom vezet, a felület hátralép** – a poszter, a backdrop és a cím a főszereplő; a vezérlők akkor jelennek meg, amikor dönteni kell.
2. **Három méterről és egy hüvelykujjal is** – minden fő út távirányítóval és egy kézzel telefonon is végigjárható.
3. **Átláthatóság a varázslat helyett** – a rendszer megmondja, mit csinál és miért (átkódolás, szkennelés, hiányzó fájl).
4. **Kép nélkül is szép** – a hiányzó artwork állapot tervezett, nem hiba.
5. **Két regiszter, egy nyelv** – előcsarnok és vetítőfülke ugyanazokból a tokenekből épül.

## Accessibility & Inclusion

- Billentyűzet / D-pad végigjárhatóság minden fő úton, nagy, kontrasztos fókuszjelzéssel.
- Minimum 44 px érintési felület; AA kontraszt szövegre (placeholder is), állapot nem csak színnel.
- `prefers-reduced-motion` tiszteletben tartva, a mozgás állapotváltást jelez, nem díszít.
- Magyar ékezetek (ő, ű) minden betűtípusban.
