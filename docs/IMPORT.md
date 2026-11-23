# Váltás Jellyfinről (import)

**Admin → Rendszer → Importálás → Jellyfin.** A Kinora átveszi a Jellyfin felhasználóit és a megnézett állapotukat.
A Jellyfin adatbázisához nem nyúl: a nyilvános HTTP API-n olvas, egy adminisztrátori API-kulccsal.

## Mi jön át

| Jellyfin | Kinora |
|---|---|
| felhasználó | fiók (az első profiljába kerül az állapot); a letiltott Jellyfin-fiókok kimaradnak |
| megnézve, lejátszások száma, utolsó lejátszás | megnézve, lejátszásszám, utolsó lejátszás (és a nézési előzmény) |
| folytatási pozíció | folytatási pozíció |
| kedvenc (film, sorozat, epizód) | kedvenc |

Ami **nem** jön át: a jelszavak (a Jellyfin API nem adja ki őket), az adminisztrátori jog, a könyvtár-hozzáférési
beállítások, a lejátszási listák és a Jellyfin saját metaadat-szerkesztései.

## Lépések

1. **Jellyfinben:** Irányítópult → API-kulcsok → új kulcs. A Kinora nem menti el a kulcsot.
2. **Kinorában:** add meg a Jellyfin címét és a kulcsot. Ha a Jellyfin más útvonalon látja ugyanazokat a fájlokat
   (például másik Docker-kötet), add meg az útvonal-leképezést soronként: `/data/movies => /media/movies`.
3. **Előnézet.** Ez semmit nem ír. Felhasználónként látszik, mi mivel párosult és mi miért nem, és a teljes lista
   letölthető CSV-ben.
4. **Importálás** a kiválasztott felhasználókra.
5. **Jelszavak:** az új fiókok jelszó nélkül jönnek létre. Mindenki kap egy egyszer használható linket (14 napig
   érvényes), azzal állítja be a saját jelszavát. A link a jelentésben és később a Felhasználók oldalon
   (**Jelszóbeállító link**) is elérhető. Amíg nincs jelszó, a fiókba nem lehet belépni.

Ha a Kinorában már van **azonos nevű fiók** (kis- és nagybetű nem számít), az állapot abba a fiókba kerül, és nem jön
létre új. A `/ \ < > " '` karakterek a névben `_` jelre cserélődnek.

## Párosítás

Elemenként az első találat számít:

1. **fájlútvonal** (az útvonal-leképezés után, pontos egyezés);
2. **TMDB-, majd IMDb-azonosító** (film és sorozat);
3. **epizódnál** a sorozat (azonosító, cím, vagy a fájl mappája alapján), azon belül évad és epizódszám; a dupla
   epizódok (`S01E03E04`) mindkét számra illeszkednek;
4. **cím és év**, de csak ha pontosan egy elem illik rá (az előnézetben ezt külön jelöli).

A TVDB-azonosítót a Kinora nem tárolja, ezért az nem használható párosításra.

## Újrafuttatás

Az import többször is lefuttatható, semmit nem duplikál:

- a Kinora megjegyzi, melyik Jellyfin-felhasználóból melyik fiók lett, átnevezés után is;
- az állapot **összefésül**: a megnézett és a kedvenc jelzés csak bekapcsolódhat, a lejátszásszám és az utolsó
  lejátszás a nagyobb értéket tartja meg, a folytatási pozíció pedig csak akkor jön át, ha a Kinorában még nincs;
- ha egy fiók még mindig jelszó nélküli, új linket kap, és a régi érvényét veszti.

---

# Váltás Sonarrról, Radarrról és Prowlarrról

**Admin → Rendszer → Importálás → Sonarr / Radarr / Prowlarr.** A Kinora az alkalmazás nyilvános API-ján olvas,
az API-kulccsal (az alkalmazásban: Beállítások → Általános), amelyet nem ment el. Előbb előnézet készül, ez
semmit nem ír. Az importálás szakaszonként választható, és a háttérben fut, folyamatjelzővel.

| Honnan | Mi lesz belőle a Kinorában |
|---|---|
| Sonarr: figyelt sorozatok | követett sorozatok: ha minden évad figyelt, „minden évad”; ha csak néhány, „kiválasztott évadok”; ha egy sem, „csak az új epizódok”. A különkiadások (0. évad) nem számítanak. |
| Radarr: figyelt, még hiányzó filmek | jóváhagyott kérések. A Kinora ugyanúgy keresi őket, mint bármely kérést. A már letöltött és a nem figyelt filmek kimaradnak. |
| minőségprofilok | Kinora-minőségprofilok: az engedélyezett felbontások (a jobb elöl), a tiltott gyenge források (CAM, TS, TC, screener), a javítás és annak határa |
| indexerek (Torznab, Newznab) | indexerek, a kategóriákkal és a prioritással |
| letöltőkliensek (qBittorrent, Transmission, Deluge, SABnzbd, NZBGet) | letöltőkliensek. Ha a Sonarr és a Radarr ugyanazt a klienst használja, egy kliens lesz belőle, a filmes és a sorozatos kategóriával. |
| gyökérmappák | Kinora-könyvtárak (útvonal-leképezéssel; a már meglévő könyvtárhoz kapcsolódik) |
| Prowlarr: indexerek | indexerek a Prowlarr saját Torznab/Newznab-csatornáján keresztül, a Prowlarr kulcsával, így azonnal működnek |

Fontos tudnivalók:

- A Sonarr és a Radarr az API-ban **elrejti a jelszavakat és az API-kulcsokat** (`********`). Az ilyen indexerek
  és kliensek kikapcsolva jönnek létre; a kulcsot a Beszerzés → Források és kliens oldalon kell megadni. A
  Kinorában megadott kulcs egy újabb importnál is megmarad.
- A sorozatok és a filmek csak akkor jöhetnek át, ha a beépített beszerzés be van kapcsolva, és van TMDB-kulcs.
  Ha a Sonarr nem adja meg a TMDB-azonosítót, a Kinora a TVDB- (vagy IMDb-) azonosító alapján keresi ki.
- Ami **nem** jön át: az egyéni formátumok (custom formats), a nyelvi szabályok, a méretkorlátok (a Kinora
  alapértékei maradnak), a távoli útvonal-leképezések, a címkék és az értesítések. A filmkérések az alapértelmezett
  minőségprofillal futnak; a sorozatok a hozzájuk rendelt profilt kapják.
- Újrafuttatáskor a Kinora frissíti, amit korábban létrehozott, és nem készít másolatot.

---

# Switching from Jellyfin (import)

**Admin → System → Import → Jellyfin** reads Jellyfin's public API with an administrator API key (the key is not
stored) and brings over users, watched state, play counts, last played dates, resume positions and favourites.
Passwords, administrator rights, library access settings and playlists are not carried over. A preview comes first
and writes nothing; new accounts get a one-time password link (14 days). Items are matched by file path (with
optional path mapping), then TMDB/IMDb id, episodes by series plus season and episode number, and last by an
exact, unique title and year. Running the import again is safe: accounts are remembered and state is merged, never
reset or duplicated.

**Sonarr / Radarr / Prowlarr** (Admin → System → Import) brings over monitored series (as followed series),
monitored films that are still missing (as approved requests), quality profiles (allowed resolutions, blocked
low-quality sources, upgrade cutoff), Torznab/Newznab indexers, download clients and root folders (as libraries);
from Prowlarr, its indexers through Prowlarr's own feeds. Sonarr and Radarr hide passwords and API keys in their API,
so such indexers and clients are created switched off until the key is entered in Kinora. Custom formats, language
rules and size limits are not carried over.
