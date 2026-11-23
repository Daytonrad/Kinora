# Kinora — Kiadási jegyzetek / Release notes

> GitHub release leírásnak készült. A funkciók a repó jelenlegi állapotát írják le; a csomagok aláírása platformonként külön lépés.

## Magyar

### Mi a Kinora?

A Kinora saját gépen vagy NAS-on futó médiaszerver filmekhez és sorozatokhoz. Saját webes felülete mellett Android-, iOS- és okostévé-kliensek készülnek hozzá. A médiatár és a fiókok a saját szervereden maradnak.

### Mit tud?

- **Filmek és sorozatok kezelése:** médiakönyvtárak beolvasása, fájlnevek és epizódok felismerése, helyi NFO-adatok és képek használata, opcionális TMDB-metaadatok, poszterek és háttérképek.
- **Böngészés és keresés:** kezdőlap folytatási és ajánlósorokkal, szűrés és rendezés, részletes film- és sorozatoldalak, kedvencek, megnézett állapot és személyes gyűjtemények.
- **Személyre szabás:** fiókonként több profil, opcionális PIN, gyerekprofil és korhatár-szűrés.
- **Lejátszás:** közvetlen lejátszás, illetve szükség esetén FFmpeg-alapú átkódolás és HLS; választható minőség, hangsávok és feliratok, lejátszási pozíció mentése, következő epizód és átugorható intró vagy stáblista.
- **Átlátható átkódolás:** a szerver teszteli a rendelkezésre álló szoftveres és hardveres kódolókat; az adminfelület és a lejátszó megmutatja a lejátszási döntés okát.
- **Ajánlások:** helyben számolt, magyarázható ajánlósorok; az ajánlórendszer nem igényel felhőszolgáltatást.
- **Gyűjtemények és címkék:** kézi és szabályalapú okosgyűjtemények, címkézés és összetett szűrés.
- **Watch party:** valós idejű közös lejátszás-szinkron, chat és reakciók a webes kliensben.
- **Letöltések:** a szerver háttérben készíthet lejátszható letöltést több minőségben. Az appokba épített offline letöltés jelenleg nem érhető el.
- **Adminisztráció:** felhasználók és meghívók, médiakönyvtárak, metaadatok, átkódolás, eszközök, adatbázis-mentés és szerverkapcsolati beállítások.
- **Saját hálózat és távoli elérés:** helyi szerverkeresés, QR-kódos eszközcsatlakozás, valamint Tailscale, Cloudflare Tunnel vagy reverse proxy használata.

### Kliensek és platformok

| Kliens | Állapot és megjegyzés |
|---|---|
| Web | A teljes webes felület böngészőből használható. A Watch party és a letöltések ezen a kliensen érhetők el. |
| Android / Android TV | Közös APK telefonra és Android TV-re; a TV-s felület távirányítóval kezelhető. |
| iOS / iPadOS | Natív SwiftUI app iOS 17+-tól; AVPlayer, kép a képben, AirPlay, zárolási képernyős vezérlés és kezdőképernyő-/zárolási képernyő-widgetek. |
| Samsung Tizen TV | Tizen TV-csomag (`.wgt`); telepítéséhez Samsung-tanúsítvánnyal történő aláírás kell. |
| LG webOS TV | webOS TV-csomag (`.ipk`). |

A kliensappok közös funkciói: szerver hozzáadása vagy keresése, jelszavas vagy QR-/kódos Quick Connect belépés, profilválasztás, könyvtárböngészés és lejátszás. A Watch party és az appon belüli offline letöltés még nem része a mobil- és TV-klienseknek.

Az iOS-kliensben jelenleg nincs feliratkezelés; a lejátszási funkciók platformonként eltérhetnek.

### Telepítés és aktuális állapot

- Ajánlott szervertelepítés: Docker Compose; Linuxon natív systemd-telepítő is rendelkezésre áll. A médiafájlok csak olvasható csatolással is használhatók.
- A mobil- és TV-csomagok aláírása eltérő platformkövetelményeket használ. A Tizen `.wgt` telepítéséhez Samsung-tanúsítvány szükséges; az iOS eszközre telepítéshez Apple-aláírás kell.
- Az alkalmazások elsődleges kezelőfelülete magyar. Ez a kiadási jegyzet angol fordítást is tartalmaz, de ez nem jelenti az app teljes angol lokalizációját.
- Chromecast-integráció és appon belüli offline letöltés nincs ebben a kiadásban.

Telepítési lépések és részletes platformleírás: [fő README](README.md) · [kliensappok útmutatója](apps/README.md).

---

## English

### What is Kinora?

Kinora is a self-hosted media server for movies and TV shows, designed to run on a home server or NAS. It includes a web client and companion clients for Android, iOS, and smart TVs. Your library and accounts stay on your own server.

### Features

- **Movie and TV library management:** scan media folders, identify files and episodes, read local NFO metadata and artwork, optionally fetch TMDB metadata, and generate posters and backdrops.
- **Browse and search:** a home screen with continue-watching and recommendation rows, filters and sorting, movie and series details, favorites, watched status, and personal collections.
- **Profiles:** multiple profiles per account, optional PINs, child profiles, and age-rating restrictions.
- **Playback:** direct play or FFmpeg transcoding with HLS when needed; quality selection, audio and subtitle tracks, saved playback position, next episode, and skippable intros or credits.
- **Visible playback decisions:** the server can test available software and hardware encoders; the admin interface and player explain why a playback or transcoding path was selected.
- **Recommendations:** locally calculated, explainable recommendations with no cloud recommendation service required.
- **Collections and tags:** manual and rule-based smart collections, tags, and advanced filtering.
- **Watch parties:** real-time synchronized playback, chat, and reactions in the web client.
- **Downloads:** the server can prepare downloadable files in several qualities in the background. Offline downloads inside the companion apps are not available yet.
- **Administration:** users and invitations, libraries, metadata, transcoding, devices, database backups, and server connection settings.
- **Home network and remote access:** local server discovery, QR-based device pairing, and support for setups using Tailscale, Cloudflare Tunnel, or a reverse proxy.

### Clients and platforms

| Client | Status and notes |
|---|---|
| Web | Full web client in a browser. Watch parties and downloads are available here. |
| Android / Android TV | One APK for phones and Android TV; the TV interface supports remote control. |
| iOS / iPadOS | Native SwiftUI app for iOS 17+ with AVPlayer, Picture in Picture, AirPlay, Lock Screen controls, and Home Screen / Lock Screen widgets. |
| Samsung Tizen TV | Tizen TV package (`.wgt`); installation requires signing with a Samsung certificate. |
| LG webOS TV | webOS TV package (`.ipk`). |

Common companion-app features include adding or discovering a server, password or QR/code-based Quick Connect sign-in, profile selection, library browsing, and playback. Watch parties and in-app offline downloads are not yet available in the mobile and TV clients.

The iOS client does not currently support subtitle selection; playback features vary by platform.

### Installation and current status

- Recommended server installation: Docker Compose. A native systemd installer is also available for Linux. Media folders can be mounted read-only.
- Mobile and TV packages have separate platform signing requirements. Tizen installation requires a Samsung certificate; installing on a physical iOS device requires Apple signing.
- Hungarian is the primary app UI language. This release note is bilingual; that does not mean the app is fully localized in English.
- Chromecast integration and offline downloads inside the companion apps are not included in this release.

For installation steps and platform details, see the [main README](README.md) and the [client app guide](apps/README.md).
