# Offline letöltés a mobilappokban

Állapot: **megvalósítva, valódi készüléken még nincs kipróbálva** (Android és iOS). Automatikus tesztek: Go (szerver),
JUnit (Android szabályok), Swift Testing (iOS szabályok). A készüléken végigjárandó lista lent van.

## Hogyan működik

1. A címlapon a letöltés gomb a szerveren elindítja az előkészítést (`POST /api/items/{id}/download`, minőség:
   eredeti, 1080p, 720p, 480p, 360p). A szerver MP4-et készít (`internal/media/download.go`), és 7 napig tartja.
2. Ha kész, a készülék letölti a háttérben:
   - **iOS:** háttér-`URLSession` (az app felfüggesztése alatt is fut, a megszakadt átvitel onnan folytatódik, ahol
     abbamaradt), Live Activity a zárolási képernyőn.
   - **Android:** a rendszer DownloadManagere (értesítéssel, az app bezárása és újraindítás után is fut), az app saját
     mappájába (`Android/data/…/files/Movies/kinora/`); tárhely-engedély nem kell.
3. A feliratok (a szöveges sávok) WebVTT-ként a videó mellé kerülnek.
4. Lejátszás kapcsolat nélkül:
   - **iOS:** a Letöltések fül; ha az app szerver nélkül indul, egy offline képernyő nyílik a letöltésekkel.
   - **Android:** az indító szerverlistáján a „Letöltések offline” rész (ha a szerver nem érhető el, ez kapja a
     fókuszt), saját lejátszóval.
5. A megnézett pozíció offline egy sorba kerül, és a következő csatlakozáskor a szerverre megy (iOS:
   `ProgressQueue`; Android: a webkliens küldi el, amikor a szerver oldala újra betöltődik). Offline a folytatás az
   offline megnézett ponttól indul.

## Profilok és gyerekprofil offline

- Egy letöltés annak a profilnak (profiloknak) a sajátja, amelyik letöltötte. A szerver a letöltés indításakor
  ellenőrzi, hogy a profil láthatja-e a címet (korhatár, könyvtár-korlátozás).
- Offline **csak az eszközön utoljára használt profil** letöltései látszanak, és profilváltás offline nincs (PIN-t és
  korhatárt csak a szerver tud ellenőrizni). Ez ugyanaz, mint online újraindításkor: az app a legutóbbi profillal folytat.
- Egy gyerekprofil így soha nem látja egy másik profil letöltését, offline sem.
- Ha két profil ugyanazt a címet tölti le, a fájl egyszer van meg; törléskor csak a saját hozzárendelés szűnik meg, a
  fájl akkor törlődik, ha már egyik profilé sem.
- iOS: a profilok rögzítése előtti letöltéseket gyerekprofil nem látja.
- Kijelentkezés, kiléptetett eszköz (elveszett munkamenet) vagy a szerver eltávolítása után az offline indulás nem
  nyitja meg a letöltéseket (iOS).

## Biztonság (Android)

Az Android-app a szerver webkliensét futtatja WebView-ban. A szerver oldalai nem kapják meg a Capacitor-hidat; az
offline letöltéshez egy szűk `window.KinoraOffline` felület kell (`KinoraOffline.java`). Hogy egy hívás mit tehet, azt a
betöltött oldal címe dönti el, nem az, amit az oldal magáról állít:

- az indító (az app saját oldala) regisztrálja, melyik szerver milyen címeken érhető el;
- egy szerver oldala csak a saját letöltéseit látja és kezeli, és **csak a saját címéről** tölthet le (abszolút URL
  azonos hosttal vagy egy útvonal; `file:`, `content:`, más host elutasítva);
- a fájlnevek csak számokból és a szerverazonosító `[a-z0-9]` részéből állnak (nincs útvonal-bejárás); feliratból
  legfeljebb 30, egyenként legfeljebb 2 MB kerül a készülékre.
- A letöltés-végpont stream-tokent is elfogad (`/api/downloads/{key}/file?st=…`), mert a DownloadManager nem ismeri a
  webkliens munkamenetét. A token csak a saját címéhez tartozó letöltést nyitja meg, 6 órán át él, és a kiléptetett
  eszköz tokenjei érvénytelenek.

Ismert korlát: a `@JavascriptInterface` egy oldalba ágyazott, más eredetű kerettől (iframe) is hívható, és a hívó
keretet nem lehet megkülönböztetni. A Kinora webkliense nem használ iframe-et.

## Tesztek

| Hol | Mit |
|---|---|
| `internal/server/download_test.go` | valódi 360p előkészítés; a kész letöltés `file_url`-je munkamenet nélkül letölthető; token nélkül 401; másik cím tokenje 404; az „eredeti” URL is tokennel jön |
| `apps/android/…/OfflineRulesTest.java` | szerver-cím felismerése, letöltés csak a saját szerverről, profil-láthatóság, törlés, biztonságos fájlnév |
| `apps/ios/KinoraKit/Tests/…/OfflineAccessTests.swift` | profil-láthatóság (gyerekprofil, régi letöltések), törlés más profilok mellett |
| `web/i18n_test.go` | a webkliens minden szövege megvan magyarul, angolul és németül |

## Kipróbálandó készüléken

**Android** (telefon, Wi-Fi-n a szerverrel):

1. Az appban egy film címlapján: ⋯ → Letöltés → „Letöltés erre az eszközre” (720p). A Letöltések oldalon az „Ezen az
   eszközön” részben: előkészítés a szerveren → letöltés %-kal → „Offline elérhető”. Közben értesítés látszik.
2. Az appot bezárva a letöltés folytatódik (értesítés).
3. Repülőgép-üzemmód, az app újraindítása: a szerverlista hibát mutat, alatta „Letöltések offline” → a film elindul,
   feliratválasztás működik, tekerés működik.
4. Félúton megállítva, újra megnyitva onnan folytatja. Repülőgép-üzemmód ki, app újra → a Kinora webes felületén a
   Folytatás sorban a film az offline pozícióval.
5. Gyerekprofillal belépve a felnőtt profil letöltése nem látszik (online a Letöltések oldalon, offline az indítóban).
6. Törlés offline és online.

**iOS** (iPhone):

1. Letöltés egy filmre és egy évadra; a háttérben (app bezárva) is halad, Live Activity a zárolási képernyőn.
2. Repülőgép-üzemmód, az app újraindítása: az offline képernyő a letöltésekkel; lejátszás, feliratok.
3. Profilváltás online egy másik profilra: a Letöltések fülön csak az ő letöltései; gyerekprofilnál a felnőtteké nem.
4. Újra online: „Újra” gomb → a Kinora kezdőlapja; a megnézett pozíció a szerveren is megjelenik.
5. Kijelentkezés után offline indításkor nem nyílnak meg a letöltések.

## Nincs benne

- Androidon a letöltés átadása a webklienstől indul: amit a szerver akkor készít el, amikor az app zárva van, a
  következő megnyitáskor kerül át a DownloadManagerhez.
- A TV-appok (Tizen, webOS, Titan) nem töltenek le; az Android TV ugyanazt az APK-t használja, ott is működik.
- Az offline lejátszó Androidon a WebView saját `<video>` lejátszója (a webkliens lejátszójának funkciói, például az
  intró átugrása, offline nincsenek meg).
