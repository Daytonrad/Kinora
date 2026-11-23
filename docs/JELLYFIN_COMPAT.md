# Jellyfin-kompatibilis API

Állapot: **megvalósítva** (`internal/jfcompat`). Automatikus tesztek: igen. Valódi kliensek: **Kodi 21.3 + JellyCon
1.1.0** kipróbálva Windowson (2026-11-23, részletek lent: „Kodi + JellyCon: teszteredmény”); az Infuse, a Swiftfin
és a Findroid még nincs kipróbálva. Az eredeti terv lent változatlanul olvasható; ahol a megvalósítás eltér tőle, a
„Döntések és eltérések” rész mondja meg.

## Döntések és eltérések (a jóváhagyás után)

- **Belépés:** `fiók` vagy `fiók/profil`, a jelszó végén `#PIN` a PIN-es profilnál. Fiók+IP páronként 5 hibás
  próbálkozás után 15 perc kizárás, és ettől függetlenül IP-nként legfeljebb 30 hiba 15 percen belül.
- **Quick Connect:** a Kinorában történő jóváhagyáskor kötelező profilt választani. PIN-es profilnál a PIN-t ott kell
  megadni, és a hibás PIN ugyanúgy beleszámít a próbálkozási korlátba, mint a hibás kód.
- **Port:** külön port, `KINORA_JELLYFIN_ADDR` (alap `:8097`), és csak `KINORA_JELLYFIN_COMPAT=1` mellett nyílik meg.
  Az Admin → Beállítások kapcsolója dönt arról, hogy válaszol-e (kikapcsolva HTTP 503).
- **Termék és verzió:** `ProductName: "Kinora"`, `Version: "10.10.7"` (API-szint). Jellyfin-logó és -branding sehol.
- **Azonosítók:** AES-sel titkosított (namespace, Kinora-ID) blokk. A kulcs a Kinora adatbázisában van
  (`jfcompat_id_key`), így újraindítás és mentés-visszaállítás után is ugyanazok az azonosítók. Visszafejtéshez nem kell
  tábla, kitalálni viszont nem lehet.
- **Útvonalak:** kis- és nagybetű-függetlenek (ahogy a Jellyfin-kliensek elvárják), és a régi `/emby` előtag is működik.
- **Lejátszás:** ha a kliens nem küld `DeviceProfile`-t (Infuse, Kodi), a Kinora az eredeti fájlt adja (direct play).
  Ha küld, a `media.Decide` dönt, és szükség esetén HLS-transcode indul a Kinora transcode-sessionjeivel (a fiókonkénti
  korláttal). A transcode a `master.m3u8` lekérésekor indul, nem a `PlaybackInfo`-nál.
- **Media Segments:** a Kinora intro-, stáblista-, recap- és előzetes-szakaszai a `/MediaSegments/{id}` végponton
  Jellyfin-típusokként (`Intro`, `Outro`, `Recap`, `Preview`) jönnek.
- **Token nélküli stream-URL:** a JellyCon a direkt stream címét (`/Videos/{id}/stream?static=True&PlaySessionId=…`)
  token nélkül adja át a Kodi lejátszójának. Az ilyen kérést a Kinora csak akkor szolgálja ki, ha a `PlaySessionId`
  egy ugyanerre az elemre kiadott `PlaybackInfo`-ból jön, és a token, amellyel azt kérték, még érvényes ugyanannak
  a profilnak (kiléptetett eszköznél tehát nem). A `PlaySessionId` 16 véletlen bájt, legfeljebb 12 óráig él a
  memóriában. A profil korhatár- és könyvtárszűrése a kiszolgáláskor is lefut.
- **Évek és címkék:** `GET /Years` és `GET /Tags` a profil által látható filmek/sorozatok alapján (a kliens
  `ParentId`-jével és `IncludeItemTypes`-ával), az `/Items` pedig szűr `Tags`-re (név) és `TagIds`-re. A címke-ID a
  címke nevéből képzett stabil hash, mert a Kinorában nincs külön címketábla. A `GET /Genres` is figyeli az
  `IncludeItemTypes`-t.
- **Felirat-mezők:** minden streamben ott van az `IsHearingImpaired` (a Kinora ezt nem tárolja; `true` csak akkor,
  ha a sáv címében „SDH” áll). A SubRip feliratok `DeliveryUrl`-je `Stream.srt`, a többi szöveges feliraté
  `Stream.vtt`, mert a JellyCon a letöltött fájlt a `Codec` alapján nevezi el.
- **Ismert korlát:** ha a kliens a transcode-ot egy kezdőpozícióval kéri (`StartTimeTicks`), a HLS-lista ettől a
  ponttól indul. Hogy a kliensek ezt hogyan kezelik (az idővonal 0-tól vagy a kezdőponttól számol), azt valódi
  kliensekkel kell ellenőrizni.

## Kliens-mátrix

| Kliens | Platform | Állapot | Megjegyzés |
|---|---|---|---|
| Infuse | Apple TV, iPhone | nincs kipróbálva | a tulajdonos teszteli, lásd a tesztlistát |
| Swiftfin | iPhone, Apple TV | nincs kipróbálva | a tulajdonos teszteli, lásd a tesztlistát |
| Kodi + JellyCon | Windows (Kodi 21.3, JellyCon 1.1.0) | **működik** | direct play teljesen; átkódolásnál nincs tekerés (lásd lent) |
| Kodi + Jellyfin for Kodi | – | nincs kipróbálva | a JellyCon-nal közös API-t használja, de külön nem teszteltük |
| Findroid | Android | nincs kipróbálva | Android-emulátor kell hozzá (nincs a gépen) |
| Jellyfin Web, Jellyfin Media Player, hivatalos Android-app | – | nem támogatott | a szerver webes felületét töltenék be |

## Kodi + JellyCon: teszteredmény

Környezet: Windows 11, Kodi 21.3, JellyCon 1.1.0 (a hivatalos `repository.jellyfin.kodi`-ból), Kinora Docker-konténer
a `testmedia` könyvtárral (3 film, 2 sorozat), `KINORA_JELLYFIN_COMPAT=1`. A Kodit JSON-RPC-n vezéreltük (a JellyCon
menüit `Files.GetDirectory`-val jártuk be, a lejátszást `Player.Open`-nel indítottuk), a Kinora oldalát a napló és a
Kinora saját API-ja mutatta.

**Működik:**

- Szerver hozzáadása címmel; Quick Connect (kód a Kodiban → jóváhagyás a Kinorában, profil- és PIN-választással; a
  hibás PIN-t elutasítja); kézi belépés felhasználónévvel és jelszóval.
- A gyerekprofillal belépve a korhatár feletti és a besorolás nélküli filmek sehol nem látszanak.
- Böngészés: könyvtárak, „Show All”, Unwatched, Recently Added, In Progress, Favorites, Genres, A–Z, Pages, Years,
  Decades, Tags, Latest, sorozat → évadok → epizódok, a globális listák (Movies / TV Shows).
- Direct play (MP4 és MKV): lejátszás, tekerés, külső magyar `.srt` felirat, hangsáv nyelve, sorozat/évad/epizód
  adatai a lejátszóban.
- Állapot: a Kodiban megállított film pozíciója a Kinorában megjelenik (Folytatás sor), és a Kinorában tárolt
  pozícióról a Kodi folytatja. A „Mark Watched” és a „Set Favourite” a Kinorában is beáll.
- A JellyCon „Skip” ablaka a Kinora intro/stáblista-szakaszaiból (`/MediaSegments`) dolgozik.
- Átkódolás („Force Transcode”): HLS (`master.m3u8` → `main.m3u8` → szegmensek) lejátszódik; leállításkor a
  Kinora leállítja az átkódolást (`DELETE /Videos/ActiveEncodings`).
- WebSocket (`/socket`): a JellyCon csatlakozik és keep-alive-ot küld.

**A teszt közben talált és javított hibák** (mindegyikre regressziós teszt készült):

| Hiba | Javítás |
|---|---|
| `/socket` mindig HTTP 500 | a naplózó `ResponseWriter` nem adta tovább a `Hijack`-et (`socket_test.go`) |
| `/Years`, `/Tags` 404 → a Years, Decades, Tags menük hibát adtak | új végpontok és `Tags`/`TagIds` szűrő (`facets_test.go`) |
| a globális „TV Shows – Latest” filmeket is mutatott | `/Items/Latest` figyeli az `IncludeItemTypes`-t (`latest_test.go`) |
| lejátszáskor `KeyError: 'IsHearingImpaired'` a JellyCon-ban | a mező minden streamben ott van (`TestPlaybackAndState`) |
| a direkt stream 401-et kapott (a JellyCon tokent nem tesz a URL-be) | `PlaySessionId`-alapú hozzáférés (lásd fent, `stream_auth_test.go`) |

**Nem működik / korlát:**

- **Átkódolásnál nincs tekerés.** A Kinora HLS-listája az átkódolással együtt nő (nincs előre teljes VOD-lista),
  ezért a Kodi `canseek: false`-t és 0 teljes hosszt jelez. Emiatt a JellyCon átkódolásnál a folytatást sem tudja
  beállítani: az elejétől indul, és a következő állapotjelentés felülírja a korábbi pozíciót. Direct playnél (ez a
  JellyCon alapértelmezése, mert nem küld `DeviceProfile`-t) a tekerés és a folytatás működik.
- Ha a Kinora adatbázisa újraindul (új telepítés), a JellyCon a régi tokent és a régi elem-ID-ket a saját
  gyorsítótárában tartja: újra be kell lépni (Change User), és a régi ID-kre épülő Kodi-listák üresek lesznek. A
  JellyCon WebSocket-kliense a Kodi indulásakor rögzíti a tokent, ezért belépés után a Kodi újraindításáig a socket
  401-et kap. Mindkettő a kliens viselkedése.

## Tesztlista: Infuse és Swiftfin

Előkészítés: a Kinorában Admin → Beállítások → **Jellyfin-kompatibilis API** be. A compose-ban a `8097:8097` port és a
`KINORA_JELLYFIN_COMPAT=1` legyen kikommentezve. Legyen egy PIN-es gyerekprofil is.

**Infuse (Apple TV, iPhone):** Beállítások → Megosztások → Megosztás hozzáadása → **Jellyfin**. Cím: a szerver IP-je,
port: `8097`.

1. Belépés `fiók` + jelszó → a könyvtárak (Filmek, Sorozatok) megjelennek, a borítók és háttérképek betöltődnek.
2. Belépés `fiók/GyerekProfil` + `jelszó#PIN` → csak a gyerekprofilnak engedett címek látszanak (a 18-as karikás film
   és a besorolás nélküliek nem).
3. Hibás PIN ötször → a hatodik, már helyes próbálkozás is elutasítva (15 percig), egy másik fiók ugyanarról a gépről
   közben beléphet.
4. Film lejátszása (H.264 MP4 és HEVC MKV is) → elindul, tekerés működik.
5. Feliratok: beágyazott és külső `.srt` (magyar ékezetekkel) → kiválasztható, helyesen jelenik meg. Hangsávváltás.
6. Félbehagyás után a Kinora webes felületén a „Folytatás” sorban ott a film, jó pozícióval; és fordítva: a Kinorában
   elkezdett film az Infuse-ban onnan folytatható.
7. Megnézettnek jelölés és kedvenc az Infuse-ban → a Kinorában is látszik (és fordítva, egy szinkronizálás után).
8. Sorozat: évadok, epizódok sorrendben, a dupla epizód egy elemként.
9. A Kinora Fiók → Eszközök listájában az Infuse eszköz látszik („Jellyfin-kompatibilis kliens”), és innen kiléptetve az
   Infuse újra belépést kér.

**Swiftfin (iPhone, Apple TV):** szerver hozzáadása: `http://<szerver-ip>:8097`.

1. A szerver megtalálható (kézi címmel, és a helyi hálózaton automatikusan is, ha a Kinora UDP 7359-en elérhető).
2. **Quick Connect:** a Swiftfin kódot mutat → a Kinora webes felületén profil menü → Eszköz csatlakoztatása → kód,
   profil (és PIN) → a Swiftfin belép, a választott profillal.
3. Az Infuse-lista 1–9. pontja.
4. Transcode: a Swiftfin natív lejátszójával (Beállítások → Videólejátszó: Native) egy HEVC/HDR film → átkódolva elindul,
   tekerés működik; a Kinora Admin → Irányítópult oldalán a futó átkódolás látszik, a lejátszás leállítása után eltűnik.
5. Intro átugrása: egy sorozatepizódnál, amelynél a Kinora talált introt, megjelenik az „Intro átugrása” gomb (ha a
   Swiftfin-verzió támogatja a Media Segments API-t).

Ha valami nem működik, a legtöbbet a Kinora naplója segít (`KINORA_LOG_LEVEL=debug`): minden kompatibilis kérés
`jellyfin-compat` sorként jelenik meg, útvonallal és státuszkóddal.

## Cél

A meglévő, natív Jellyfin-kliensek (Infuse, Swiftfin, Findroid, Streamyfin, a Kodi Jellyfin-bővítményei, a Jellyfin
Android TV app) a Kinorához is tudjanak csatlakozni: böngészés, keresés, lejátszás (direct play és transcode),
feliratok, folytatás, megnézett és kedvenc állapot, Quick Connect.

Ez **részleges** kompatibilitás. A Kinora nem lesz Jellyfin, és nem cél a teljes API.

### Nem célok

- **Webes alapú kliensek:** Jellyfin Web, Jellyfin Media Player, és a hivatalos Android (telefon) app, amely a
  szerver webes felületét tölti be. Ezekhez a szervernek a `jellyfin-web` alkalmazást kellene kiszolgálnia. Azt nem
  másoljuk (GPL), és nem is szállítjuk.
- Élő TV, zene, könyvek, fotók, SyncPlay, távvezérlés (`/Sessions/.../Command`), trickplay, dalszövegek.
- Az admin-, plugin-, ütemező- és szerverbeállítási API-k, valamint a felhasználókezelés. Ezekre a Kinora saját
  felülete való.
- Lejátszási listák és gyűjtemények írása (olvasásuk később jöhet).

### Szabályok

- **Kód:** külön csomag (`internal/jfcompat`). A Jellyfin kódjából semmit nem veszünk át, csak a nyilvános HTTP
  API-viselkedést valósítjuk meg: a nyilvános API-leírás és a kliensek tényleges kérései alapján.
- **Alapból kikapcsolva:** a `KINORA_JELLYFIN_COMPAT=1` környezeti változó **és** az admin-beállítás
  (Admin → Beállítások) együtt kapcsolja be.
- **Jogosultságok:** minden Kinora-szabály itt is érvényes. Gyerekprofil és korhatár (`RatingSQL`), a fiók
  könyvtár-korlátozása, és a tokenek a Kinora eszközlistájában jelennek meg (onnan visszavonhatók).

## Hálózat: külön port

**Javaslat:** külön HTTP-port, alapból `8097` (`KINORA_JELLYFIN_ADDR`), ugyanabban a folyamatban, külön `ServeMux`-on.

- **Miért külön port:** a Jellyfin-kliensek a szerver gyökeréből indulnak ki (`/System/Info/Public`, `/Items`…),
  ami ütközne a Kinora saját webes felületével. Külön porton a két API nem keveredik, kikapcsolt állapotban nincs
  nyitva semmi, és egy reverse proxy külön (al)domainre teheti.
- **Alternatíva:** ugyanazon a porton egy alútvonal (`/jellyfin`). A Jellyfin „Base URL” beállítása miatt sok
  kliens ezt is kezeli, de nem mind. Ezért nem ez a javaslat.
- **LAN-felfedezés:** a Jellyfin-kliensek UDP 7359-en `who is JellyfinServer?` üzenettel keresnek. A Kinora
  ugyanezen a porton figyel (a saját `who is KinoraServer?` üzenetére). Bekapcsolt kompatibilitásnál a Jellyfin-féle
  kérdésre is válaszol, a kompatibilis port címével.

## Felhasználó ↔ profil (döntés kell)

A Jellyfinben a felhasználó egyben a „néző”. A Kinorában egy fiókhoz legfeljebb 8 profil tartozik (gyerekprofil,
PIN, korhatár). Ezt valahogy le kell képezni.

**A) javaslat: egy Jellyfin-belépés = egy Kinora-profil.**
- Felhasználónévként a fiók neve (az első nem gyerek profil), vagy `fiók/profil` (pl. `kati/Gyerek`) adható meg.
  A jelszó a fiók jelszava. PIN-es profilnál a jelszó végére `#PIN` kerül, mert a kliensekben nincs külön PIN-mező.
- A kiadott token a profilhoz van rögzítve (a Kinora ezt már tudja: `profile_lock`). A gyerekprofil szűrése így
  minden kérésre automatikusan érvényes.
- A Quick Connect kódot a Kinorában jóváhagyó felhasználó maga választja ki, melyik profiljával lép be a kliens
  (a Kinora Quick Connectje ezt már most így csinálja).
- `GET /Users/Public` (a kliensek bejelentkező képernyője) alapból üres listát ad. Bekapcsolva a nem gyerek
  profilokat is mutathatja.

**B) egyszerűbb: egy Jellyfin-belépés = egy Kinora-fiók**, mindig az első profillal. Ez kevesebb munka, de egy
családban mindenki ugyanazt a „Folytatás” listát látná, és a gyerekprofil csak külön fiókkal lenne használható.

## Azonosítók

- **Elemek:** a Jellyfin-kliensek 32 jegyű hexadecimális (GUID-szerű) azonosítót várnak. Ez a Kinora belső
  `int64` azonosítójának kulcsolt HMAC-je (szerverenként titkos kulcs), és egy táblában vissza is fejthető.
- **Miért kitalálhatatlan:** a kliensek a képeket (`/Items/{id}/Images/Primary`) **token nélkül** kérik. A
  képvégpont ezért nyilvános, de csak az kérheti le, aki egy jogosult listázásból már ismeri az azonosítót. Egy
  egyszerű számláló alapú ID mellett bárki végigpróbálhatná a rejtett elemek borítóit is.
- **Könyvtárak, évadok, felhasználók és szerver:** ugyanígy, külön névtérrel. A `ServerId` a Kinora meglévő
  szerverazonosítója.

## Hitelesítés

- Elfogadott fejlécek, mert a kliensek eltérnek:
  - `Authorization: MediaBrowser Client=…, Device=…, DeviceId=…, Version=…, Token=…`
  - `X-Emby-Authorization`
  - `X-Emby-Token`, `X-MediaBrowser-Token`
  - `api_key` lekérdezési paraméter (a streameknél).
- A token Kinora eszköz-session (`platform = "jellyfin"`, eszköznév a `Device` mezőből). Megjelenik az
  Admin → Eszközök és a saját eszközlista oldalon, és onnan kiléptethető.
- A belépés ugyanazt a rate limitet kapja, mint a Kinora saját belépése (IP-nként és peerenként).

## Végpontok (minimális készlet)

A ✱ jelű végpontokat a kliensek indításkor hívják. Ezek nélkül el sem jutnak a bejelentkezésig.

| Terület | Végpontok | Kinora-megfelelő |
|---|---|---|
| Szerver ✱ | `GET /System/Info/Public`, `GET /System/Info`, `GET /System/Ping`, `POST /System/Ping`, `GET /Branding/Configuration` | `/api/status`, beállítások |
| Belépés ✱ | `POST /Users/AuthenticateByName`, `GET /Users/Public`, `POST /Sessions/Logout` | `/api/login` + profilválasztás |
| Quick Connect | `GET /QuickConnect/Enabled`, `POST /QuickConnect/Initiate`, `GET /QuickConnect/Connect`, `POST /Users/AuthenticateWithQuickConnect` | a meglévő Quick Connect |
| Felhasználó ✱ | `GET /Users/Me`, `GET /Users/{id}` (Policy, Configuration: nyelvek, „kihagyások”) | profil (hang- és feliratnyelv) |
| Nézetek ✱ | `GET /UserViews`, `GET /Users/{id}/Views` | könyvtárak (`CollectionType`: `movies` / `tvshows`; a vegyes könyvtár `mixed`) |
| Listázás | `GET /Items`, `GET /Users/{id}/Items` (ParentId, IncludeItemTypes, Recursive, SortBy/SortOrder, StartIndex/Limit, SearchTerm, Filters=IsFavorite/IsResumable/IsPlayed/IsUnplayed, Genres, Years, Ids, Fields) | `store.Browse` / `QueryItems` + `RatingSQL` |
| Részletek | `GET /Items/{id}`, `GET /Users/{id}/Items/{itemId}` | `ItemFull` |
| Kezdőlap | `GET /Items/Latest`, `GET /Users/{id}/Items/Latest`, `GET /UserItems/Resume`, `GET /Users/{id}/Items/Resume`, `GET /Shows/NextUp` | `RecentlyAdded`, `ContinueWatching`, `NextUp` |
| Sorozatok | `GET /Shows/{id}/Seasons`, `GET /Shows/{id}/Episodes` | `Children` |
| Keresés | `GET /Search/Hints` (és `/Items?SearchTerm=`) | `Search` |
| Képek | `GET /Items/{id}/Images/{type}[/{index}]` (Primary, Backdrop, Thumb; `maxWidth`/`fillWidth` → átméretezés vagy eredeti) | `handleImage` lánca |
| Lejátszás | `POST /Items/{id}/PlaybackInfo` (DeviceProfile → MediaSources), `GET /Videos/{id}/stream[.{ext}]?static=true`, `GET /Items/{id}/Download`, `GET /Videos/{id}/master.m3u8`, `GET /Videos/{id}/main.m3u8`, szegmensek | `media.Decide`, `/api/stream`, HLS-session |
| Feliratok | `GET /Videos/{id}/{mediaSourceId}/Subtitles/{index}/Stream.{vtt,srt}` | `Subs.VTT`, külső feliratfájl |
| Állapot | `POST /Sessions/Playing`, `/Sessions/Playing/Progress`, `/Sessions/Playing/Stopped`; `POST`/`DELETE /UserPlayedItems/{id}` és `/Users/{uid}/PlayedItems/{id}`; `POST`/`DELETE /UserFavoriteItems/{id}` és `/Users/{uid}/FavoriteItems/{id}` | `ReportProgress`, `SetPlayed`, `SetFlag` |
| Kliens-kellékek | `POST /Sessions/Capabilities/Full`, `GET`/`POST /DisplayPreferences/{id}`, `GET /Localization/Cultures`, `GET /Localization/Options` | üres vagy állandó válasz |
| WebSocket | `/socket` | elfogadja, keep-alive-ot küld, mást nem (a kliensek enélkül is működnek, de egyesek újrapróbálkoznak) |
| Évek, címkék (megvalósítás közben) | `GET /Genres`, `GET /Years`, `GET /Tags` | `store.FacetsOf` (a profil láthatósági szűrőjével) |

A régi (`/Users/{id}/…`) és az új (10.9 utáni, `userId` paraméteres) útvonalpárokat is kiszolgálja, mert a kliensek
vegyesen használják őket.

### Elemek leképezése (BaseItemDto)

| Jellyfin | Kinora |
|---|---|
| `Type`: `Movie`, `Series`, `Season`, `Episode`, `CollectionFolder` | `kind` |
| `Name`, `SortName`, `Overview`, `ProductionYear`, `PremiereDate`, `OfficialRating`, `CommunityRating`, `Genres`, `Tags` | megfelelő mezők (a profil metaadat-nyelvén) |
| `RunTimeTicks` (100 ns) | `runtime_sec × 10⁷` |
| `SeriesId`, `SeriesName`, `SeasonId`, `ParentId`, `IndexNumber`, `ParentIndexNumber`, `IndexNumberEnd` | sorozat/évad/epizódszámok (dupla epizód is) |
| `ProviderIds` (`Tmdb`, `Imdb`) | `tmdb_id`, `imdb_id` (a negatív/0 értékek kimaradnak) |
| `UserData` (`Played`, `PlaybackPositionTicks`, `PlayCount`, `IsFavorite`, `LastPlayedDate`, `UnplayedItemCount`) | `user_data` |
| `ImageTags`, `BackdropImageTags` | a képfájl módosítási idejéből képzett tag (a kliensek cache-kulcsa) |
| `MediaSources`, `MediaStreams` (Video/Audio/Subtitle: Codec, Language, IsDefault, IsForced, IsExternal, IsTextSubtitleStream, Channels, Width/Height, VideoRange) | `Streams`, `Container`, `Bitrate`, `HDR` |
| `Chapters` | fejezetek; az intro- és stáblista-szakaszok (a Kinora sajátja) ide nem kerülnek. A kliensek a Jellyfin „Media Segments” API-ját használják, ez a 2. körben jöhet. |

### Lejátszási döntés

A kliens `DeviceProfile`-ból (DirectPlayProfiles, TranscodingProfiles, CodecProfiles) Kinora-`Caps` lesz
(konténerek, video- és audiokodekek, HDR), és ugyanaz a `media.Decide` dönt, mint a Kinora saját appjaiban:

- **direct:** `SupportsDirectPlay` és `SupportsDirectStream` = true, a `Path`/stream URL-lel. Az Infuse és a Kodi
  többnyire így játszik.
- **remux/transcode:** `TranscodingUrl` a kompatibilis porton (`/Videos/{id}/master.m3u8?...`). Mögötte a Kinora
  meglévő HLS-sessionje fut, a felhasználónkénti transcode-korláttal (1. fázis).
- A hangsáv- és feliratválasztás (`AudioStreamIndex`, `SubtitleStreamIndex`) a Kinora típuson belüli indexeire
  képeződik. A képalapú felirat beégetése (`SubtitleMethod=Encode`) a meglévő beégetési útra kerül.
- A Kinora indokai („miért transzkódol”) a `TranscodeReasons` mezőbe nem férnek bele, mert az egy rögzített
  felsorolás. Ezért a legközelebbi értékek kerülnek oda (pl. `VideoCodecNotSupported`).

## Verziószám és termék

A kliensek ellenőrzik a szerver verzióját: a Swiftfin és a Findroid 10.8 vagy 10.9 alatt nem csatlakozik. A
`/System/Info/Public` válaszában ezért egy Jellyfin-API-szintet kell jelezni (pl. `"Version": "10.10.7"`), mert
különben a kliensek elutasítják a szervert.

- **Javaslat:** `ProductName: "Kinora"`, `ServerName`: a Kinora szerverneve, `Version`: a megvalósított
  API-szint. Ha egy kliens a `ProductName`-re is szűr, azt a valódi kliensekkel kell kideríteni. Ebben az esetben
  sem írunk „Jellyfin Server”-t: dokumentáljuk, hogy az a kliens nem támogatott.

## Tesztelés

1. **Egység- és integrációs tesztek** (`httptest`) minden végpontra, rögzített kliens-kérésekkel (a valódi kliensek
   fejléceivel és paramétereivel). Külön jogosultsági tesztek: gyerekprofil, könyvtár-korlátozás, idegen elem-ID,
   lejárt/visszavont token, és a képvégpont rejtett elemre.
2. **Valódi kliensek** (a fázis vége, dokumentálva, mi működik és mi nem):
   - ezen a gépen elérhető: a **Kodi** Windowsra a Jellyfin-bővítménnyel, és a **Findroid** Android-emulátorban
     (ha az Android SDK elérhető);
   - a te eszközeiden: **Swiftfin** és **Infuse** (iPhone, Apple TV). Ehhez egy rövid tesztlistát adok, mit
     érdemes kipróbálni.
3. Az eredmény a `docs/JELLYFIN_COMPAT.md` „Kliens-mátrix” táblázatába kerül.

## Ütemezés (javasolt sorrend)

1. Váz: külön port, kapcsoló, auth-fejlécek, ID-leképezés, `System/Info`, belépés, Quick Connect, `Users/Me`.
2. Böngészés: nézetek, `Items` (szűrők, rendezés, lapozás), részletek, képek, kezdőlap, sorozatok, keresés.
3. Lejátszás: `PlaybackInfo`, direct stream, HLS, feliratok, állapotjelentés, megnézett és kedvenc.
4. Tesztelés valódi kliensekkel, javítások, a kliens-mátrix kitöltése.

## Döntések, amelyekhez a jóváhagyásod kell

1. **Felhasználó ↔ profil:** A (profil szintű belépés, `fiók/profil` és `#PIN`), vagy B (fiók szintű, első profil)?
   Javaslat: **A**.
2. **Port:** külön port (alap `8097`), vagy alútvonal (`/jellyfin`) ugyanazon a porton? Javaslat: **külön port**.
3. **Verzió/termék:** `ProductName: "Kinora"`, `Version`: egy Jellyfin-API-szint (pl. `10.10.7`). Ha egy kliens
   ezzel nem működik, nem támogatjuk (nem adjuk ki magunkat Jellyfinnek). Rendben?
4. **Kliensek:** melyik a legfontosabb neked? A Swiftfint és az Infuse-t csak te tudod iPhone-on vagy Apple TV-n
   kipróbálni.
