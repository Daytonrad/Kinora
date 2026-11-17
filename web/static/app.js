/* Kinora web client – vanilla JS, hash routing, no build step. */
'use strict';

// ===================================================================== strings
// Every user-visible text lives here so an English translation can be added later.
const T = {
  appName: 'Kinora',
  home: 'Kezdőlap', collections: 'Gyűjtemények', downloads: 'Letöltések', admin: 'Admin', searchPh: 'Keresés címre, leírásra, címkére',
  switchProfile: 'Profilváltás', account: 'Fiók', logout: 'Kijelentkezés', joinParty: 'Csatlakozás watch partyhoz',
  setupTitle: 'Állítsuk be a Kinorát', setupLede: 'Add meg a szerver nevét, és hozd létre az adminisztrátori fiókot.',
  serverName: 'Szerver neve', username: 'Felhasználónév', password: 'Jelszó', password2: 'Jelszó még egyszer',
  createAdmin: 'Admin fiók létrehozása', pwMismatch: 'A két jelszó nem egyezik.',
  loginTitle: 'Bejelentkezés', loginLede: 'Lépj be a fiókodba.', login: 'Belépés',
  joinTitle: 'Regisztráció meghívóval', joinLede: s => `Meghívást kaptál ide: ${s}. Válassz felhasználónevet és jelszót.`,
  join: 'Fiók létrehozása', inviteInvalid: 'Ez a meghívó érvénytelen vagy lejárt. Kérj újat attól, aki küldte.',
  whoWatches: 'Ki néz?', addProfile: 'Profil hozzáadása', editProfiles: 'Profilok szerkesztése', done: 'Kész',
  kids: 'Gyerek', pinTitle: n => `${n} profilja PIN-nel védett`, pinLabel: 'PIN', enter: 'Belépés',
  profileName: 'Név', color: 'Szín', pin: 'PIN (4–8 számjegy, üresen hagyva nincs)', pinKeep: 'Hagyd üresen, ha nem változik',
  removePin: 'PIN eltávolítása', isKids: 'Gyerekprofil (csak korhatár-besorolt, korosztálynak megfelelő tartalom)',
  maxRating: 'Korhatár-szint', ratingLevels: ['Mindenki (G, KN)', '6+ / PG', '12+ / PG-13', '16+ / R', '18+'], noLimit: 'Nincs korlát',
  subLangs: 'Feliratnyelvek (prioritás)', audioLangs: 'Hangsávnyelvek (prioritás)', langHint: 'Vesszővel elválasztva, pl. hun,eng',
  subMode: 'Feliratok', subModes: { auto: 'Automatikus (idegen nyelvű hangnál)', always: 'Mindig', off: 'Kikapcsolva' },
  autoplayNext: 'Következő rész automatikus indítása', maxBitrate: 'Maximális bitráta', bitrateOpts: [[0, 'Eredeti'], [20000, '20 Mbps'], [8000, '8 Mbps'], [4000, '4 Mbps'], [2000, '2 Mbps'], [1000, '1 Mbps']],
  save: 'Mentés', cancel: 'Mégse', delete: 'Törlés', close: 'Bezárás', edit: 'Szerkesztés', create: 'Létrehozás', copy: 'Másolás', copied: 'Vágólapra másolva.',
  saved: 'Mentve.', deleted: 'Törölve.', confirmDelete: n => `Biztosan törlöd: ${n}?`,
  play: 'Lejátszás', resume: t => `Folytatás (${t})`, playEp: (s, e) => `Lejátszás: ${s}. évad ${e}. rész`, resumeEp: (s, e) => `Folytatás: ${s}. évad ${e}. rész`,
  markPlayed: 'Megnézettnek jelölés', markUnplayed: 'Megnézetlennek jelölés', favorite: 'Kedvenc', hide: 'Elrejtés', unhide: 'Elrejtés visszavonása',
  hiddenToast: 'Elrejtve. Ha újra elkezded nézni, visszakerül.', hideSeries: 'Sorozat elrejtése a listákból',
  addToCollection: 'Hozzáadás gyűjteményhez', newCollection: 'Új gyűjtemény…', inCollections: 'Gyűjteményekben', addedTo: n => `Hozzáadva: ${n}`, removedFrom: n => `Eltávolítva innen: ${n}`,
  tags: 'Címkék', addTag: 'Új címke…', download: 'Letöltés', downloadQ: 'Minőség', original: 'Eredeti fájl', startParty: 'Watch party indítása',
  fixMatch: 'Találat javítása', similar: 'Hasonló', seasons: 'Évadok', episodes: 'Epizódok', season: n => n === 0 ? 'Különkiadások' : `${n}. évad`,
  ep: (s, e) => `S${String(s).padStart(2, '0')}E${String(e).padStart(2, '0')}`, min: m => `${m} perc`, hm: (h, m) => h ? `${h} ó ${m} p` : `${m} perc`,
  genres: 'Műfajok', file: 'Fájl', video: 'Videó', audio: 'Hang', subtitles: 'Felirat',
  all: 'Összes', more: 'Összes', moreActions: 'Műveletek', moreBtn: 'Továbbiak', details: 'Részletek', data: 'Adatok',
  libEmpty: 'Ebben a könyvtárban még nincs semmi. Ha most adtad hozzá, a szkennelés pár perc.', noResults: 'Nincs találat ezekkel a szűrőkkel.',
  filters: 'Szűrők', sort: 'Rendezés', asc: 'Növekvő', desc: 'Csökkenő', yearFrom: 'Évtől', yearTo: 'Évig', minRating: 'Min. értékelés',
  watched: 'Megnézve', unwatched: 'Nem nézett', any: 'Mind', onlyFav: 'Csak kedvencek', clear: 'Szűrők törlése', saveSmart: 'Mentés smart gyűjteményként',
  homeEmptyTitle: 'Még üres a vetítőterem', homeEmptyAdmin: 'Adj hozzá egy könyvtárat az adminfelületen, és a Kinora beolvassa a filmjeidet.', homeEmptyUser: 'Az adminisztrátor még nem adott hozzá könyvtárat.',
  addLibrary: 'Könyvtár hozzáadása', searchTitle: q => `Találatok: „${q}”`, searchEmpty: 'Nincs találat.',
  collEmpty: 'Még nincs gyűjteményed. A kézi gyűjteménybe te válogatsz, a smart gyűjtemény szabályok alapján frissül magától.',
  newManual: 'Új kézi gyűjtemény', newSmart: 'Új smart gyűjtemény', manual: 'Kézi', smart: 'Smart', shared: 'Közös (az egész háztartás látja)',
  pinned: 'Kitűzés a kezdőlapra', collName: 'Név', rules: 'Szabályok', matchAll: 'Minden szabály teljesüljön', matchAny: 'Bármelyik szabály teljesüljön',
  addRule: 'Szabály hozzáadása', limit: 'Legfeljebb (üres = nincs korlát)', preview: n => `Előnézet – ${n} találat`, removeFromColl: 'Eltávolítás',
  ops: { is: 'az', is_not: 'nem az', gte: 'legalább', lte: 'legfeljebb', contains: 'tartalmazza', not_contains: 'nem tartalmazza', starts_with: 'így kezdődik', within_days: 'az utóbbi N napban', older_than_days: 'régebben, mint N nap' },
  yes: 'Igen', no: 'Nem', moveUp: 'Feljebb', moveDown: 'Lejjebb',
  dlEmpty: 'Nincs letöltés. A részletes oldalon választhatsz minőséget, az átkódolás a háttérben fut.', dlStatus: { queued: 'Sorban áll', running: 'Készül', done: 'Kész', error: 'Hiba' },
  dlStarted: 'A letöltés elindult. A Letöltések oldalon követheted.', saveFile: 'Mentés eszközre', dlKept: 'A kész fájlok 7 napig érhetők el.',
  // player
  back: 'Vissza', pause: 'Szünet', rewind: '10 mp vissza', forward: '10 mp előre', mute: 'Némítás', fullscreen: 'Teljes képernyő',
  audioTrack: 'Hangsáv', subtitle: 'Felirat', subOff: 'Kikapcsolva', showAll: 'Összes nyelv mutatása', burnIn: 'beégetve', quality: 'Minőség',
  qAuto: 'Automatikus / eredeti', why: 'Miért így megy?', whyTitle: { direct: 'Közvetlen lejátszás', remux: 'Remux (csomagolásváltás)', transcode: 'Átkódolás' },
  encoder: 'Encoder', nextEp: 'Következő rész', nextIn: n => `Indul ${n} mp múlva`, playNow: 'Indítás most', dismiss: 'Mégse',
  playErr: 'A lejátszás nem sikerült', retry: 'Újra', forceTranscode: 'Próbáld átkódolva', loading: 'Betöltés…', unknownLang: 'ismeretlen',
  // party
  party: 'Watch party', partyCode: 'Kód', partyLink: 'Meghívó link', partyMembers: 'Nézők', partyEveryone: 'Mindenki irányíthat', partyChatPh: 'Üzenet…',
  partySend: 'Küldés', partyWaiting: names => `Várakozás: ${names.join(', ')}…`, partyJoinTitle: 'Csatlakozás watch partyhoz', partyCodeLabel: '6 karakteres kód',
  partyNotFound: 'Nincs ilyen watch party. Ellenőrizd a kódot.', partyLost: 'A kapcsolat megszakadt, újracsatlakozás…', host: 'házigazda', buffering: 'tölt…',
  // admin
  aDash: 'Irányítópult', aLibs: 'Könyvtárak', aUsers: 'Felhasználók', aInvites: 'Meghívók', aTranscode: 'Átkódolás', aMeta: 'Metaadat', aConnect: 'Csatlakozás', aSettings: 'Beállítások',
  version: 'Verzió', uptime: 'Futásidő', titles: 'Cím', files: 'Fájl', size: 'Méret', activeTranscodes: 'Aktív átkódolás', parties: 'Watch party', scans: 'Szkennelés',
  libName: 'Név', libPath: 'Mappa', libKind: 'Típus', kinds: { movies: 'Filmek', shows: 'Sorozatok', mixed: 'Vegyes' }, choose: 'Tallózás…', scan: 'Szkennelés',
  lastScan: 'Utolsó szkennelés', never: 'még soha', phase: { walk: 'bejárás', probe: 'elemzés', cleanup: 'rendrakás', done: 'kész' }, scanErr: 'Szkennelési hiba',
  pickFolder: 'Mappa kiválasztása', useFolder: 'Ezt a mappát választom', up: 'Feljebb', mediaHint: 'Dockerben a médiád a /media alatt van (csak olvasható).',
  newUser: 'Új felhasználó', isAdmin: 'Adminisztrátor', resetPw: 'Új jelszó', profilesN: n => `${n} profil`, you: '(te)',
  newInvite: 'Új meghívó', note: 'Megjegyzés', uses: 'Felhasználások', validHours: 'Érvényesség (óra, 0 = korlátlan)', inviteLink: 'Meghívó link', usesLeft: n => `még ${n}×`, expires: 'Lejár',
  hwTest: 'Encoder-önteszt futtatása', hwTesting: 'Teszt fut…', selectedEnc: 'Használt encoder', encSetting: 'Encoder kiválasztása', encAuto: 'Automatikus (első működő hardveres, különben szoftveres)', tonemap: 'HDR → SDR tónusleképezés',
  tmdbKey: 'TMDB API kulcs (v3 kulcs vagy v4 token)', metaLang: 'Metaadat nyelve', refreshMeta: 'Metaadat frissítése', retryNoMatch: 'A „nincs találat” elemek újrapróbálása', metaStatus: 'Állapot',
  metaHint: 'Kulcs nélkül a Kinora csak a fájlnevekből és a helyi NFO-fájlokból dolgozik. Kulcsot ingyen kérhetsz: themoviedb.org → Settings → API.',
  publicUrl: 'Nyilvános URL', scanInterval: 'Szkennelési intervallum (perc, 0 = ki)', backup: 'Adatbázis-mentés letöltése', backupHint: 'Konzisztens pillanatkép (VACUUM INTO) – futás közben is biztonságos.',
  envLocked: v => `környezeti változóból beállítva (${v})`,
  connectLede: 'Ezeken a címeken érhető el a szerver. Telefonról olvasd be a QR-kódot.', noPublic: 'Nincs megadva nyilvános URL (Beállítások).',
  matchSearch: 'Keresés a TMDB-ben', matchApply: 'Ezt választom', tmdbId: 'vagy TMDB azonosító',
  netErr: 'Nem érem el a szervert. Ellenőrizd a kapcsolatot.',
  // devices & quick connect
  connectDevice: 'Eszköz csatlakoztatása', qcLede: 'Írd be a tévén vagy telefonon látható 6 jegyű kódot.', qcCode: '6 jegyű kód', next: 'Tovább',
  qcWants: (n, p) => `${n || 'Ismeretlen eszköz'} (${p}) szeretne belépni a fiókodba.`, qcProfile: 'Melyik profillal lépjen be?', qcAnyProfile: 'Profilválasztó az eszközön',
  qcPinNote: 'A PIN-nel védett profil PIN-jét az új eszközön kell megadni.', qcKidsNote: 'A gyerekprofillal belépett eszköz nem válthat más profilra.',
  approve: 'Jóváhagyás', deny: 'Elutasítás', qcApproved: 'Kész! Az eszköz pár másodpercen belül belép.', qcDenied: 'Elutasítva.', qcKids: 'Gyerekprofilból nem lehet eszközt csatlakoztatni.',
  myDevices: 'Bejelentkezett eszközök', thisDevice: 'ez az eszköz', signOutDevice: 'Kiléptetés', signOutOthers: 'Kiléptetés mindenhonnan (ezen kívül)',
  signedOutN: n => `${n} eszköz kiléptetve.`, lastActive: 'Utoljára aktív', aDevices: 'Eszközök',
  platforms: { web: 'Böngésző', android: 'Android', androidtv: 'Android TV', ios: 'iOS', tizen: 'Samsung TV', webos: 'LG TV' },
  // folders & permissions
  permCheck: 'Jogosultság ellenőrzése', permOk: 'Olvasható – rendben.', permFix: 'Javítás (futtasd a szerveren):', permGroup: 'Vagy csoporton keresztül:',
  locked: 'nincs jog', hostPath: p => `A hoston: ${p}`, shortcuts: 'Gyorsugrás',
  mediaHintNative: 'Bármelyik mappa választható, amit a Kinora szolgáltatás felhasználója olvashat; a lakat jelzi, ahová nincs joga.',
  mediaHintDocker: 'Dockerben a /media alatt vannak a csatolt mappák; a docker-compose.host.yml-lel a /host alatt a teljes gép is.',
  // skip intro / credits
  skipIntro: 'Intró átugrása', skipRecap: 'Előzmények átugrása', skipCredits: 'Stáblista átugrása', skippedIntro: 'Intró átugorva',
  skipMode: 'Intró és stáblista', skipModes: { manual: 'Átugrás gombbal', auto: 'Automatikus átugrás', off: 'Kikapcsolva' },
  markIntroStart: 'Intró eleje itt', markIntroEnd: 'Intró vége itt', markCredits: 'Stáblista kezdete itt', clearIntro: 'Kézi intró-jelölés törlése',
  markStartFirst: 'Előbb jelöld meg az intró elejét.', markSaved: 'Jelölés mentve.',
  propagateAsk: 'Alkalmazod az évad többi részére is, ahol ugyanez a főcím szól?', propagated: n => `${n} további résznél beállítva.`,
  aSegments: 'Intró-felismerés', segAnalyze: 'Intró-felismerés futtatása', segQueued: n => `Sorba állítva: ${n} évad.`,
  segBackend: 'Ujjlenyomat-készítő', segQueue: 'Várakozó évadok', segRunning: 'Most fut', segStats: (e, a, i, c, m) => `${a}/${e} rész elemezve · intró: ${i} · stáblista: ${c} · kézi jelölés: ${m}`,
  segDetection: 'Intró- és stáblista-felismerés', segWindow: 'Intró keresése az első N percben (2–30)',
  qcEnabled: 'Belépés kóddal (Quick Connect)', discEnabled: 'Szerverkeresés a helyi hálózaton (UDP 7359)', on: 'Bekapcsolva', off: 'Kikapcsolva',
  ago: s => { const m = Math.round(s / 60); if (m < 1) return 'most'; if (m < 60) return `${m} perce`; const h = Math.round(m / 60); if (h < 48) return `${h} órája`; return `${Math.round(h / 24)} napja`; },
};

// ===================================================================== helpers
const $ = (s, r = document) => r.querySelector(s);
const app = () => document.getElementById('app');

/** h('div.class', {attr|onEvent}, ...children) – tiny DOM builder (text is always escaped). */
const tok = n => parseFloat(getComputedStyle(document.documentElement).getPropertyValue(n)) || 0;
function h(tag, props, ...kids) {
  const [t, ...cls] = tag.split('.');
  const e = document.createElement(t || 'div');
  if (cls.length) e.className = cls.join(' ');
  if (props) {
    for (const [k, v] of Object.entries(props)) {
      if (v == null || v === false) continue;
      if (k.startsWith('on') && typeof v === 'function') e.addEventListener(k.slice(2), v);
      else if (k === 'class') e.className = (e.className ? e.className + ' ' : '') + v;
      else if (k === 'style' && typeof v === 'object') Object.assign(e.style, v);
      else if (['value', 'checked', 'disabled', 'selected', 'textContent', 'htmlFor'].includes(k)) e[k] = v;
      else e.setAttribute(k, v === true ? '' : v);
    }
  }
  add(e, kids);
  return e;
}
/** put(el, ...kids) – replaceChildren that drops false/null/undefined/'' (no raw "false" on screen). */
function put(el, ...kids) { el.textContent = ""; return add(el, kids); }
function add(e, kids) {
  for (const k of kids.flat(Infinity)) {
    if (k == null || k === false || k === '') continue;
    e.append(k instanceof Node ? k : document.createTextNode(String(k)));
  }
  return e;
}
const ICONS = {
  play: 'M8 5v14l11-7z', pause: 'M7 5h4v14H7zM13 5h4v14h-4z', back: 'M15 5l-7 7 7 7', more: 'M12 7a1.5 1.5 0 110-3 1.5 1.5 0 010 3zm0 6.5a1.5 1.5 0 110-3 1.5 1.5 0 010 3zm0 6.5a1.5 1.5 0 110-3 1.5 1.5 0 010 3z',
  check: 'M5 12.5l4.5 4.5L19 7.5', heart: 'M12 20s-7-4.4-7-10a4 4 0 017-2.6A4 4 0 0119 10c0 5.6-7 10-7 10z', star: 'M12 3.5l2.6 5.3 5.9.9-4.3 4.1 1 5.8-5.2-2.7-5.2 2.7 1-5.8-4.3-4.1 5.9-.9z',
  rw: 'M11 7L5 12l6 5zM19 7l-6 5 6 5z', ff: 'M13 7l6 5-6 5zM5 7l6 5-6 5z', vol: 'M5 9h4l5-4v14l-5-4H5zM17 9a4 4 0 010 6', mute: 'M5 9h4l5-4v14l-5-4H5zM17 9l4 6M21 9l-4 6',
  fs: 'M4 9V4h5M20 9V4h-5M4 15v5h5M20 15v5h-5', cc: 'M4 6h16v12H4zM8 10.5a2 2 0 100 3M14 10.5a2 2 0 100 3', gear: 'M12 9a3 3 0 100 6 3 3 0 000-6zM4 12h2M18 12h2M12 4v2M12 18v2M6.3 6.3l1.4 1.4M16.3 16.3l1.4 1.4M6.3 17.7l1.4-1.4M16.3 7.7l1.4-1.4',
  info: 'M12 3a9 9 0 100 18 9 9 0 000-18zM12 11v6M12 7.5v.5', next: 'M6 6l8 6-8 6zM17 6v12', chat: 'M4 5h16v11H9l-5 4z', eye: 'M2 12s4-7 10-7 10 7 10 7-4 7-10 7S2 12 2 12zM12 9a3 3 0 100 6 3 3 0 000-6z',
  eyeOff: 'M3 3l18 18M10.6 6.1A10 10 0 0112 6c6 0 10 6 10 6a17 17 0 01-3 3.5M6.6 7.6C3.8 9.3 2 12 2 12s4 7 10 7a9.5 9.5 0 004.4-1.1', plus: 'M12 5v14M5 12h14', dl: 'M12 4v11M7 10l5 5 5-5M5 20h14',
  lock: 'M7 11V8a5 5 0 0110 0v3M5 11h14v10H5z', people: 'M9 11a3.5 3.5 0 100-7 3.5 3.5 0 000 7zM2 20c0-4 3-6 7-6s7 2 7 6M16 4.5a3.5 3.5 0 010 6.5M18 14c2.5.6 4 2.6 4 6', tag: 'M3 12V4h8l10 10-8 8z M7.5 8.5h.01',
  x: 'M6 6l12 12M18 6L6 18', filter: 'M4 5h16l-6 8v6l-4-2v-4z',
};
function icon(name, fill) {
  const svg = document.createElementNS('http://www.w3.org/2000/svg', 'svg');
  svg.setAttribute('viewBox', '0 0 24 24');
  svg.setAttribute('aria-hidden', 'true');
  const p = document.createElementNS('http://www.w3.org/2000/svg', 'path');
  p.setAttribute('d', ICONS[name] || '');
  const solid = fill ?? ['play', 'pause', 'next', 'star'].includes(name);
  p.setAttribute('fill', solid ? 'currentColor' : 'none');
  p.setAttribute('stroke', 'currentColor');
  p.setAttribute('stroke-width', solid ? '0' : '2');
  p.setAttribute('stroke-linecap', 'round');
  p.setAttribute('stroke-linejoin', 'round');
  svg.append(p);
  return svg;
}
function toast(msg, err) {
  const box = document.getElementById('toasts');
  for (const old of [...box.children]) if (old.textContent === msg) old.remove();
  while (box.children.length >= 3) box.firstChild.remove();
  const t = h('div.toast' + (err ? '.err' : ''), { role: err ? 'alert' : 'status' }, msg);
  box.append(t);
  setTimeout(() => t.remove(), err ? 6000 : 3200);
}
const fmtTime = s => {
  s = Math.max(0, Math.floor(s || 0));
  const hh = Math.floor(s / 3600), mm = Math.floor(s / 60) % 60, ss = s % 60;
  return (hh ? hh + ':' + String(mm).padStart(2, '0') : mm) + ':' + String(ss).padStart(2, '0');
};
const fmtRuntime = sec => { if (sec < 60) return `${Math.round(sec)} mp`; const m = Math.round(sec / 60); return T.hm(Math.floor(m / 60), m % 60); };
const fmtBytes = b => { if (!b) return '0 B'; const u = ['B', 'KB', 'MB', 'GB', 'TB']; const i = Math.min(4, Math.floor(Math.log(b) / Math.log(1024))); return (b / 1024 ** i).toFixed(i ? 1 : 0).replace('.', ',') + ' ' + u[i]; };
const resLabel = (w, hgt) => (hgt >= 1600 || w >= 3200) ? '4K' : (hgt >= 900 || w >= 1800) ? '1080p' : (hgt >= 650 || w >= 1200) ? '720p' : hgt ? 'SD' : '';
const imgURL = (it, kind) => `/api/images/${it.id}/${kind}?v=${it.updated_at || 0}`;
const debounce = (f, ms) => { let t; return (...a) => { clearTimeout(t); t = setTimeout(() => f(...a), ms); }; };
const qrImg = text => {
  try { const q = qrcode(0, 'M'); q.addData(text); q.make(); return h('div.qr', null, h('img', { src: q.createDataURL(5, 2), alt: 'QR: ' + text })); }
  catch { return h('div'); }
};
async function copyText(t) {
  try { await navigator.clipboard.writeText(t); toast(T.copied); }
  catch { prompt(T.copy, t); }
}

// ===================================================================== API
class HTTPError extends Error { constructor(status, msg, body) { super(msg); this.status = status; this.body = body; } }
async function api(method, path, body, opts = {}) {
  let res;
  try {
    res = await fetch(path, {
      method, credentials: 'same-origin', keepalive: !!opts.keepalive,
      headers: body !== undefined ? { 'Content-Type': 'application/json' } : {},
      body: body !== undefined ? JSON.stringify(body) : undefined,
    });
  } catch (e) {
    if (!opts.quiet) toast(T.netErr, true);
    throw e;
  }
  let data = null;
  const ct = res.headers.get('content-type') || '';
  if (ct.includes('json')) data = await res.json().catch(() => null);
  if (!res.ok) {
    const msg = (data && data.error) || `HTTP ${res.status}`;
    if (res.status === 401 && !opts.noRedirect) { state.me = null; go('#/login'); }
    else if (res.status === 428 && !opts.noRedirect) { go('#/profiles'); }
    else if (!opts.quiet) toast(msg, true);
    throw new HTTPError(res.status, msg, data);
  }
  return data;
}
const GET = (p, o) => api('GET', p, undefined, o);
const POST = (p, b = {}, o) => api('POST', p, b, o);

// ===================================================================== state & router
const state = { status: null, me: null, facets: null, libs: null, cleanup: [] };
function go(hash) { if (location.hash === hash) route(); else location.hash = hash; }
function onCleanup(f) { state.cleanup.push(f); }
function parseHash() {
  const raw = location.hash.slice(1) || '/';
  const [path, qs] = raw.split('?');
  return { parts: path.split('/').filter(Boolean), q: new URLSearchParams(qs || '') };
}

// a deep link (e.g. #/quickconnect?code=…) survives the login and profile screens
function rememberAfter(hash) { try { sessionStorage.setItem('kinora.after', hash); } catch { /* storage blocked */ } }
function takeAfter() {
  let v = null;
  try { v = sessionStorage.getItem('kinora.after'); sessionStorage.removeItem('kinora.after'); } catch { /* storage blocked */ }
  return v;
}

async function loadMe() {
  try { state.me = await GET('/api/me', { noRedirect: true, quiet: true }); }
  catch { state.me = null; }
  return state.me;
}

async function route() {
  for (const f of state.cleanup.splice(0)) { try { f(); } catch { /* ignore */ } }
  document.querySelectorAll('.modal-back').forEach(m => m.remove());
  const { parts, q } = parseHash();
  const [p0, p1, p2] = parts;
  if (!state.status) state.status = await GET('/api/status', { quiet: true }).catch(() => ({}));
  if (state.status.setup_needed) { if (p0 !== 'setup') return go('#/setup'); return viewSetup(); }
  if (p0 === 'setup') return go('#/login');
  if (p0 === 'join' && p1) return viewJoin(p1);
  if (!state.me) await loadMe();
  if (!state.me) {
    if (p0 !== 'login') { if (p0 === 'quickconnect') rememberAfter(location.hash); return go('#/login'); }
    return viewLogin();
  }
  if (p0 === 'login') return go('#/profiles');
  if (p0 === 'quickconnect' && state.me.profile) return viewQuickConnect(q.get('code') || '');
  if (p0 === 'quickconnect') rememberAfter(location.hash);
  if (p0 === 'profiles' || !state.me.profile) return viewProfiles(p1 === 'edit');
  document.title = T.appName;
  try {
    switch (p0) {
      case undefined: case 'home': return await viewHome();
      case 'library': return await viewLibrary(+p1, q);
      case 'item': return await viewItem(+p1);
      case 'play': return await viewPlayer(+p1, q);
      case 'search': return await viewSearch(q.get('q') || '');
      case 'collections': return p1 === 'new' ? await viewCollectionEdit(null, q) : p2 === 'edit' ? await viewCollectionEdit(+p1) : p1 ? await viewCollection(+p1) : await viewCollections();
      case 'downloads': return await viewDownloads();
      case 'party': return await viewPartyJoin(p1);
      case 'admin': return await viewAdmin(p1 || 'dash');
      case 'account': return await viewAccount();
      default: return go('#/home');
    }
  } catch (e) {
    if (!(e instanceof HTTPError)) console.error(e);
  }
}
// D-pad / arrow-key navigation: move focus to the nearest focusable element in that direction.
function spatialNav(e) {
  const dir = { ArrowLeft: 'l', ArrowRight: 'r', ArrowUp: 'u', ArrowDown: 'd' }[e.key];
  if (!dir || e.altKey || e.ctrlKey || e.metaKey) return;
  const a = document.activeElement;
  if (a && a.matches('input, textarea, select, [contenteditable], .timeline, video')) return;
  if (document.querySelector('.player') && !document.querySelector('.modal-back')) return; // the player owns its keys
  const scope = document.querySelector('.modal-back .modal') || document.querySelector('.menu-pop.floating') || document;
  const focusables = [...scope.querySelectorAll('a[href], button, input, select, textarea, [tabindex]:not([tabindex="-1"])')]
    .filter(el => el !== a && !el.disabled && el.offsetParent !== null && !el.closest('.hidden'));
  if (!a || a === document.body || !scope.contains(a)) {
    const first = scope.querySelector('.card, .who-item') || focusables[0];
    if (first) { e.preventDefault(); first.focus(); }
    return;
  }
  const r = a.getBoundingClientRect();
  const cx = (r.left + r.right) / 2, cy = (r.top + r.bottom) / 2;
  let best = null, bs = Infinity;
  for (const el of focusables) {
    const b = el.getBoundingClientRect();
    if (!b.width && !b.height) continue;
    const dx = (b.left + b.right) / 2 - cx, dy = (b.top + b.bottom) / 2 - cy;
    let main, cross;
    if (dir === 'l') { if (b.right > r.left + 1) continue; main = -dx; cross = Math.abs(dy); }
    else if (dir === 'r') { if (b.left < r.right - 1) continue; main = dx; cross = Math.abs(dy); }
    else if (dir === 'u') { if (b.bottom > r.top + 1) continue; main = -dy; cross = Math.abs(dx); }
    else { if (b.top < r.bottom - 1) continue; main = dy; cross = Math.abs(dx); }
    const score = main + cross * 2.5;
    if (score < bs) { bs = score; best = el; }
  }
  if (best) { e.preventDefault(); best.focus(); best.scrollIntoView({ block: 'nearest', inline: 'nearest' }); }
}
document.addEventListener('keydown', spatialNav);
window.addEventListener('hashchange', route);
window.addEventListener('DOMContentLoaded', route);

// ===================================================================== layout
function shell(active, ...content) {
  const me = state.me, p = me.profile;
  const root = app();
  put(root);
  const libsNav = h('span');
  const searchInput = h('input', { type: 'search', placeholder: T.searchPh, 'aria-label': T.searchPh, value: active === 'search' ? (parseHash().q.get('q') || '') : '' });
  searchInput.addEventListener('input', debounce(() => {
    const v = searchInput.value.trim();
    if (v.length >= 2) location.hash = '#/search?q=' + encodeURIComponent(v);
  }, 300));
  searchInput.addEventListener('keydown', e => { if (e.key === 'Enter' && searchInput.value.trim()) location.hash = '#/search?q=' + encodeURIComponent(searchInput.value.trim()); });
  const menuPop = h('div.menu-pop.hidden', { role: 'menu' },
    h('button', { role: 'menuitem', onclick: () => POST('/api/profiles/leave').then(() => { state.me.profile = null; go('#/profiles'); }) }, T.switchProfile),
    h('a', { role: 'menuitem', href: '#/party' }, T.joinParty),
    h('a', { role: 'menuitem', href: '#/account' }, T.account),
    !p.is_kids && !me.profile_locked && h('a', { role: 'menuitem', href: '#/quickconnect' }, T.connectDevice),
    h('hr'),
    h('button', { role: 'menuitem', onclick: () => POST('/api/logout').then(() => { state.me = null; go('#/login'); }) }, T.logout));
  const avatarBtn = h('button.avatar', { style: { background: p.color }, 'aria-label': p.name, 'aria-haspopup': 'menu', onclick: e => { e.stopPropagation(); menuPop.classList.toggle('hidden'); } }, p.name.slice(0, 1).toUpperCase());
  const closeMenu = () => menuPop.classList.add('hidden');
  document.addEventListener('click', closeMenu);
  onCleanup(() => document.removeEventListener('click', closeMenu));
  const top = h('header.top' + (active === 'home' ? '' : '.solid'), null,
    h('a.logo', { href: '#/home' }, T.appName),
    h('nav.nav', { 'aria-label': 'Fő menü' },
      h('a' + (active === 'home' ? '.on' : ''), { href: '#/home' }, T.home),
      libsNav,
      h('a' + (active === 'collections' ? '.on' : ''), { href: '#/collections' }, T.collections),
      h('a' + (active === 'downloads' ? '.on' : ''), { href: '#/downloads' }, T.downloads),
      me.user.is_admin && h('a' + (active === 'admin' ? '.on' : ''), { href: '#/admin' }, T.admin)),
    h('div.search-box', null, searchInput),
    h('div.menu', null, avatarBtn, menuPop));
  const main = h('main', null, content);
  root.append(top, main);
  (state.libs ? Promise.resolve(state.libs) : GET('/api/libraries').then(l => (state.libs = l))).then(libs => {
    libsNav.replaceWith(...libs.map(l => h('a' + (active === 'lib' + l.id ? '.on' : ''), { href: '#/library/' + l.id }, l.name)));
  }).catch(() => {});
  window.scrollTo(0, 0);
  return main;
}

function modal(title, body, actions) {
  const back = h('div.modal-back', { role: 'dialog', 'aria-modal': 'true', 'aria-label': title });
  const close = () => { back.remove(); document.removeEventListener('keydown', esc); };
  const esc = e => { if (e.key === 'Escape') close(); };
  document.addEventListener('keydown', esc);
  back.addEventListener('mousedown', e => { if (e.target === back) close(); });
  const box = h('div.modal', null, h('h2', null, title), body, actions && h('div.foot', null, actions(close)));
  back.append(box);
  document.body.append(back);
  setTimeout(() => (box.querySelector('input,select,button') || box).focus(), 30);
  return { close, box };
}
function confirmBox(text) {
  return new Promise(res => {
    modal(text, null, close => [h('button.btn', { onclick: () => { close(); res(false); } }, T.cancel), h('button.btn.primary', { onclick: () => { close(); res(true); } }, T.delete)]);
  });
}

// ===================================================================== auth views
function authPage(title, lede, form) {
  put(app(), h('div.center', null, h('div.auth', null, h('span.logo', null, T.appName), h('h1', null, title), h('p.lede', null, lede), form)));
  setTimeout(() => form.querySelector('input')?.focus(), 30);
}
function field(label, input, hint) { return h('label.field', null, h('span', null, label), input, hint && h('div.hint', { class: 'mt1' }, hint)); }

function viewSetup() {
  const name = h('input', { type: 'text', value: 'Kinora', autocomplete: 'off' });
  const user = h('input', { type: 'text', autocomplete: 'username' });
  const pw = h('input', { type: 'password', autocomplete: 'new-password' });
  const pw2 = h('input', { type: 'password', autocomplete: 'new-password' });
  const form = h('form', { onsubmit: async e => {
    e.preventDefault();
    if (pw.value !== pw2.value) return toast(T.pwMismatch, true);
    await POST('/api/setup', { server_name: name.value, username: user.value, password: pw.value });
    state.status = null; await loadMe(); go('#/profiles');
  } }, field(T.serverName, name), field(T.username, user), field(T.password, pw), field(T.password2, pw2),
  h('button.btn.primary.big', { type: 'submit', class: 'wide' }, T.createAdmin));
  authPage(T.setupTitle, T.setupLede, form);
}

function viewLogin() {
  const user = h('input', { type: 'text', autocomplete: 'username' });
  const pw = h('input', { type: 'password', autocomplete: 'current-password' });
  const form = h('form', { onsubmit: async e => {
    e.preventDefault();
    await POST('/api/login', { username: user.value, password: pw.value }, { noRedirect: true });
    await loadMe(); go('#/profiles');
  } }, field(T.username, user), field(T.password, pw), h('button.btn.primary.big', { type: 'submit', class: 'wide' }, T.login));
  authPage(T.loginTitle, (state.status.server_name || T.appName) + ' – ' + T.loginLede, form);
}

async function viewJoin(code) {
  let info;
  try { info = await GET('/api/invites/' + encodeURIComponent(code), { quiet: true, noRedirect: true }); }
  catch { return authPage(T.joinTitle, T.inviteInvalid, h('a.btn', { href: '#/login' }, T.loginTitle)); }
  const user = h('input', { type: 'text', autocomplete: 'username' });
  const pw = h('input', { type: 'password', autocomplete: 'new-password' });
  const pw2 = h('input', { type: 'password', autocomplete: 'new-password' });
  const form = h('form', { onsubmit: async e => {
    e.preventDefault();
    if (pw.value !== pw2.value) return toast(T.pwMismatch, true);
    await POST('/api/join', { code, username: user.value, password: pw.value }, { noRedirect: true });
    await loadMe(); go('#/profiles');
  } }, field(T.username, user), field(T.password, pw), field(T.password2, pw2), h('button.btn.primary.big', { type: 'submit', class: 'wide' }, T.join));
  authPage(T.joinTitle, T.joinLede(info.server_name), form);
}

// ===================================================================== profiles
const COLORS = ['#c97c5d', '#b5a27a', '#8fa37a', '#7fa6a0', '#c2b4a3', '#a3826b', '#b07a7a', '#8a9a8f'];

async function viewProfiles(editMode) {
  await loadMe();
  const me = state.me;
  const grid = h('div.who-grid');
  for (const p of me.profiles) {
    grid.append(h('button.who-item', { onclick: () => editMode ? profileEditor(p) : selectProfile(p), 'aria-label': p.name },
      h('div.face', { style: { background: p.color } }, p.name.slice(0, 1).toUpperCase(), p.has_pin && h('span.lock', null, icon('lock'))),
      h('span', null, p.name), p.is_kids && h('small', null, T.kids)));
  }
  const kidsNow = me.profile && me.profile.is_kids;
  if (me.profiles.length < 8 && !kidsNow) grid.append(h('button.who-item', { onclick: () => profileEditor(null) }, h('div.face.add', null, icon('plus')), h('span', null, T.addProfile)));
  put(app(), h('div.center', null, h('div.who', null,
    h('h1', null, editMode ? T.editProfiles : T.whoWatches), grid,
    h('div', { class: 'mt6' }, !kidsNow && h('a.btn.ghost', { href: editMode ? '#/profiles' : '#/profiles/edit' }, editMode ? T.done : T.editProfiles)))));
  grid.querySelector('button')?.focus();
}

async function selectProfile(p) {
  const done = async () => { await loadMe(); state.facets = null; go(takeAfter() || '#/home'); };
  if (!p.has_pin) { await POST(`/api/profiles/${p.id}/select`, {}); return done(); }
  const pin = h('input.pin-input', { type: 'password', inputmode: 'numeric', maxlength: 8, autocomplete: 'off', 'aria-label': T.pinLabel });
  const m = modal(T.pinTitle(p.name), h('form', { onsubmit: async e => {
    e.preventDefault();
    try { await POST(`/api/profiles/${p.id}/select`, { pin: pin.value }); m.close(); done(); }
    catch { pin.value = ''; pin.focus(); }
  } }, pin, h('div.foot', null, h('button.btn.primary', { type: 'submit' }, T.enter))));
}

function profileEditor(p) {
  const isNew = !p;
  p = p || { name: '', color: COLORS[Math.floor(Math.random() * COLORS.length)], is_kids: false, max_rating_level: null, sub_langs: 'hun,eng', audio_langs: 'hun,eng', sub_mode: 'auto', autoplay_next: true, max_bitrate: 0 };
  let color = p.color;
  const name = h('input', { type: 'text', value: p.name, maxlength: 30 });
  const sw = h('div.swatches', null, COLORS.map(c => h('button' + (c === color ? '.on' : ''), { type: 'button', style: { background: c }, 'aria-label': c, onclick: e => { color = c; sw.querySelectorAll('button').forEach(b => b.classList.remove('on')); e.currentTarget.classList.add('on'); } })));
  const pin = h('input', { type: 'password', inputmode: 'numeric', maxlength: 8, placeholder: p.has_pin ? T.pinKeep : '' });
  const removePin = h('input', { type: 'checkbox' });
  const kids = h('input', { type: 'checkbox', checked: p.is_kids });
  const lvl = h('select', null, h('option', { value: '-1' }, T.noLimit), T.ratingLevels.map((l, i) => h('option', { value: i, selected: p.max_rating_level === i }, l)));
  const subs = h('input', { type: 'text', value: p.sub_langs });
  const auds = h('input', { type: 'text', value: p.audio_langs });
  const mode = h('select', null, Object.entries(T.subModes).map(([k, v]) => h('option', { value: k, selected: p.sub_mode === k }, v)));
  const auto = h('input', { type: 'checkbox', checked: p.autoplay_next });
  const skipSel = h('select', null, Object.entries(T.skipModes).map(([k, v]) => h('option', { value: k, selected: (p.skip_intro || 'manual') === k }, v)));
  const br = h('select', null, T.bitrateOpts.map(([v, l]) => h('option', { value: v, selected: p.max_bitrate === v }, l)));
  const body = h('div', null, field(T.profileName, name), h('label.field', null, h('span', null, T.color), sw), field(T.pin, pin),
    p.has_pin && h('label.check', null, removePin, T.removePin),
    h('label.check', null, kids, T.isKids), field(T.maxRating, lvl),
    h('div.grid2', null, field(T.subLangs, subs, T.langHint), field(T.audioLangs, auds, T.langHint)),
    h('div.grid2', null, field(T.subMode, mode), field(T.maxBitrate, br)), field(T.skipMode, skipSel), h('label.check', null, auto, T.autoplayNext));
  modal(isNew ? T.addProfile : p.name, body, close => [
    !isNew && state.me.profiles.length > 1 && h('button.btn.danger', { class: 'push', onclick: async () => {
      if (!(await confirmBox(T.confirmDelete(p.name)))) return;
      await api('DELETE', '/api/profiles/' + p.id); close(); viewProfiles(true);
    } }, T.delete),
    h('button.btn', { onclick: close }, T.cancel),
    h('button.btn.primary', { onclick: async () => {
      const body = { name: name.value, color, is_kids: kids.checked, max_rating_level: +lvl.value, sub_langs: subs.value, audio_langs: auds.value, sub_mode: mode.value, autoplay_next: auto.checked, max_bitrate: +br.value, skip_intro: skipSel.value };
      if (pin.value) body.pin = pin.value; else if (removePin.checked) body.pin = '';
      if (isNew) await POST('/api/profiles', body); else await api('PATCH', '/api/profiles/' + p.id, body);
      close(); toast(T.saved); viewProfiles(!isNew);
    } }, T.save)]);
}

// ===================================================================== cards & shelves
function itemSub(it) {
  if (it.kind === 'episode') return [it.series_title, it.season_num != null && it.episode_num != null ? T.ep(it.season_num, it.episode_num) : ''].filter(Boolean).join('  ');
  if (it.kind === 'series') return [it.year, it.episode_count ? `${it.episode_count} rész` : ''].filter(Boolean).join('  ');
  return it.year || '';
}
function artImg(it, kind, alt) {
  const wrap = h('div.art');
  const ph = h('div.ph', null, it.kind === 'episode' && it.series_title ? it.series_title : it.title, it.year && it.kind !== 'episode' ? h('small', null, it.year) : null);
  const img = h('img', { src: imgURL(it, kind), alt: alt || '', loading: 'lazy', decoding: 'async' });
  img.addEventListener('error', () => { img.replaceWith(ph); wrap.classList.add('noart'); });
  wrap.append(img);
  return wrap;
}
function card(it, style = 'poster', opts = {}) {
  const art = artImg(it, style === 'poster' ? 'poster' : (it.kind === 'episode' ? 'thumb' : 'backdrop'), it.title);
  const ud = it.user_data || {};
  if (style === 'landscape' && ud.position > 0 && it.runtime_sec) art.append(h('div.prog', null, h('i', { style: { width: Math.min(100, ud.position / it.runtime_sec * 100) + '%' } })));
  if (ud.played && it.kind !== 'season') art.append(h('span.tick', { title: T.watched }, icon('check')));
  else if (it.kind === 'series' && it.unplayed_count) art.append(h('span.count', { title: 'megnézetlen' }, it.unplayed_count));
  const href = '#/item/' + it.id;
  const el = h('div.card.' + style, { tabindex: 0, role: 'link', 'aria-label': it.title, onclick: e => { if (!e.target.closest('.more')) location.hash = opts.playOnClick ? '#/play/' + it.id : href; }, onkeydown: e => { if (e.key === 'Enter') location.hash = opts.playOnClick ? '#/play/' + it.id : href; } },
    art, h('div.cap', null, h('b', null, it.title), h('span', null, itemSub(it)), opts.reason && h('span.why', null, opts.reason)));
  el._item = it; el._reason = opts.reason || '';
  if (opts.menu) {
    const btn = h('button.more', { 'aria-label': T.moreActions, onclick: e => { e.stopPropagation(); opts.menu(it, btn); } }, icon('more', true));
    el.append(btn);
  }
  return el;
}
function popMenu(anchor, entries) {
  document.querySelectorAll('.menu-pop.floating').forEach(m => m.remove());
  const r = anchor.getBoundingClientRect();
  const close = () => { pop.remove(); document.removeEventListener('mousedown', off); };
  const pop = h('div.menu-pop.floating', { role: 'menu', style: { position: 'fixed', top: (r.bottom + tok('--s2')) + 'px', left: Math.min(r.left, innerWidth - tok('--col-side')) + 'px', right: 'auto' } },
    entries.filter(Boolean).map(([label, fn]) => h('button', { role: 'menuitem', onclick: () => { close(); fn(); } }, label)));
  document.body.append(pop);
  const off = e => { if (!pop.contains(e.target)) close(); };
  setTimeout(() => document.addEventListener('mousedown', off), 0);
  pop.querySelector('button')?.focus();
  onCleanup(close);
}
function shelf(title, items, style, opts = {}) {
  const track = h('div.shelf-track.' + style, { role: 'list' }, items.map(it => card(it, style, { menu: opts.menu, reason: opts.reasons && opts.reasons[it.id], playOnClick: opts.playOnClick })));
  return h('section.shelf', { 'aria-label': title }, h('div.shelf-head', null, h('h2', null, title), opts.more && h('a', { href: opts.more }, T.more)), track);
}

// ===================================================================== home
async function viewHome() {
  const main = shell('home');
  const data = await GET('/api/home');
  const hideMenu = hideAs => (it, btn) => popMenu(btn, [
    [hideAs === 'series' ? T.hideSeries : T.hide, async () => {
      const id = hideAs === 'series' && it.series_id ? it.series_id : it.id;
      await POST(`/api/items/${id}/hide`, { hidden: true }); toast(T.hiddenToast); viewHome();
    }],
    [T.markPlayed, async () => { await POST(`/api/items/${it.id}/played`, { played: true }); viewHome(); }],
    ['Részletek', () => go('#/item/' + it.id)],
  ]);
  if (!data.rows.length) {
    const admin = state.me.user.is_admin;
    main.append(h('div.empty', { class: 'pt9' }, h('h1', null, T.homeEmptyTitle), h('p', null, admin ? T.homeEmptyAdmin : T.homeEmptyUser),
      admin && h('a.btn.primary', { href: '#/admin/libraries' }, T.addLibrary)));
    return;
  }
  let hero = null;
  if (data.hero) { hero = heroBlock(data.hero, data.rows); main.append(hero); }
  for (const row of data.rows) {
    const landscape = row.style === 'landscape';
    main.append(shelf(row.title, row.items, row.style, { more: row.more, reasons: row.reasons, playOnClick: landscape, menu: hideMenu(row.hide_as) }));
  }
  // the beam follows focus: a poster case lit by the remote becomes tonight's feature on the facade
  if (hero) {
    let t;
    main.addEventListener('focusin', e => {
      const c = e.target.closest('.card');
      if (!c || !c._item || !main.contains(c)) return;
      clearTimeout(t);
      t = setTimeout(() => fillHero(hero, c._item, c._reason), tok('--t-out'));
    });
    onCleanup(() => clearTimeout(t));
  }
}
function heroBlock(it, rows) {
  let reason = '';
  for (const r of rows) if (r.reasons && r.reasons[it.id]) { reason = r.reasons[it.id]; break; }
  const sec = h('section.hero', null, h('div.hero-img'), h('div.hero-body'));
  sec._shown = 0;
  fillHero(sec, it, reason);
  return sec;
}
/** fillHero paints one title onto the facade (the initial hero, and whichever case the remote lights up). */
function fillHero(sec, it, reason) {
  if (sec._shown === it.id) return;
  sec._shown = it.id;
  const bg = sec.querySelector('.hero-img');
  const img = new Image();
  bg.classList.add('swap');
  img.onload = () => { if (sec._shown !== it.id) return; bg.style.backgroundImage = 'url("' + img.src + '")'; bg.classList.remove('swap'); };
  img.onerror = () => { if (sec._shown !== it.id) return; bg.style.backgroundImage = 'none'; bg.classList.remove('swap'); };
  img.src = imgURL(it, it.kind === 'episode' ? 'thumb' : 'backdrop');
  const ud = it.user_data || {};
  const title = it.kind === 'episode' && it.series_title ? it.series_title : it.title;
  const sub = it.kind === 'episode' ? T.ep(it.season_num, it.episode_num) + ' · ' + it.title : '';
  reason = reason || '';
  put(sec.querySelector('.hero-body'),
    reason && h('div.reason', null, reason[0].toUpperCase() + reason.slice(1)),
    h('h1', null, title),
    h('div.meta', null, sub && h('span', null, sub), it.year && h('span', null, it.year), it.content_rating && h('span.badge', null, it.content_rating), it.runtime_sec > 0 && it.kind !== 'series' && h('span', null, fmtRuntime(it.runtime_sec)), it.genres && it.genres.length && h('span.muted', null, it.genres.slice(0, 3).join(', '))),
    it.overview && h('p', null, it.overview),
    h('div.actions', null,
      h('button.btn.primary.big', { onclick: () => it.kind === 'series' ? go('#/item/' + it.id) : go('#/play/' + it.id) }, icon('play'), ud.position > 0 ? T.resume(fmtTime(ud.position)) : T.play),
      h('a.btn.big', { href: '#/item/' + it.id }, icon('info'), T.details)));
}

// ===================================================================== search
async function viewSearch(q) {
  const main = shell('search');
  main.append(h('div.page-head', null, h('h1', null, T.searchTitle(q))));
  const items = await GET('/api/search?q=' + encodeURIComponent(q));
  main.append(items.length ? h('div.grid', null, items.map(it => card(it, 'poster'))) : h('p.empty', null, T.searchEmpty));
  setTimeout(() => { const s = $('.search-box input'); if (s) { s.focus(); s.setSelectionRange(s.value.length, s.value.length); } }, 0);
}

// ===================================================================== library browser
async function facets(lib) { return GET('/api/facets' + (lib ? '?library=' + lib : '')); }

async function viewLibrary(libId, q) {
  const main = shell('lib' + libId);
  const libs = state.libs || (state.libs = await GET('/api/libraries'));
  const lib = libs.find(l => l.id === libId);
  const f = await facets(libId);
  state.facets = f;
  const sel = { genre: new Set(q.getAll('genre')), tag: new Set(q.getAll('tag')), year_from: q.get('year_from') || '', year_to: q.get('year_to') || '', min_rating: q.get('min_rating') || '',
    played: q.get('played') || '', favorite: q.get('favorite') === 'true', sort: q.get('sort') || 'title', order: q.get('order') || (q.get('sort') === 'added' ? 'desc' : 'asc'), q: q.get('q') || '' };
  const grid = h('div.grid');
  const countEl = h('span.muted');
  const sentinel = h('div', { class: 'sentinel' });
  let offset = 0, total = 0, loading = false, gen = 0;

  const params = () => {
    const p = new URLSearchParams({ library: libId, sort: sel.sort, order: sel.order });
    sel.genre.forEach(g => p.append('genre', g)); sel.tag.forEach(t => p.append('tag', t));
    for (const k of ['year_from', 'year_to', 'min_rating', 'played', 'q']) if (sel[k]) p.set(k, sel[k]);
    if (sel.favorite) p.set('favorite', 'true');
    return p;
  };
  const load = async reset => {
    if (loading && !reset) return;
    if (reset) { offset = 0; put(grid); gen++; }
    const my = gen;
    if (!reset && offset >= total) return;
    loading = true;
    const p = params(); p.set('offset', offset); p.set('limit', 60);
    try {
      const r = await GET('/api/items?' + p);
      if (my !== gen) return;
      total = r.total; offset += r.items.length;
      countEl.textContent = `${total} cím`;
      grid.append(...r.items.map(it => card(it, 'poster')));
      if (!total) grid.append(h('p.empty', null, lib && lib.count === 0 ? T.libEmpty : T.noResults));
    } finally { loading = false; }
  };
  const refresh = () => {
    const p = params(); p.delete('library');
    history.replaceState(null, '', '#/library/' + libId + '?' + p);
    load(true);
  };
  const chipList = (list, set) => h('div.chips', null, list.slice(0, 60).map(x => {
    const c = h('button.chip' + (set.has(x.value) ? '.on' : ''), { type: 'button', 'aria-pressed': set.has(x.value) ? 'true' : 'false', onclick: () => {
      set.has(x.value) ? set.delete(x.value) : set.add(x.value);
      c.classList.toggle('on'); c.setAttribute('aria-pressed', set.has(x.value)); refresh();
    } }, x.value, h('span.n', null, x.count));
    return c;
  }));
  const inp = (key, ph, type = 'number') => { const i = h('input', { type, placeholder: ph, value: sel[key], 'aria-label': ph }); i.addEventListener('input', debounce(() => { sel[key] = i.value.trim(); refresh(); }, 350)); return i; };
  const playedSel = h('select', { 'aria-label': T.watched, onchange: e => { sel.played = e.target.value; refresh(); } },
    [['', T.any], ['false', T.unwatched], ['true', T.watched]].map(([v, l]) => h('option', { value: v, selected: sel.played === v }, l)));
  const fav = h('input', { type: 'checkbox', checked: sel.favorite, onchange: e => { sel.favorite = e.target.checked; refresh(); } });
  const filters = h('aside.filters', { 'aria-label': T.filters },
    f.genres.length > 0 && [h('h2', null, T.genres), chipList(f.genres, sel.genre)],
    f.tags.length > 0 && [h('h2', null, T.tags), chipList(f.tags, sel.tag)],
    h('h2', null, 'Év'), h('div.grid2', null, inp('year_from', T.yearFrom), inp('year_to', T.yearTo)),
    h('h2', null, T.minRating), inp('min_rating', 'pl. 7'),
    h('h2', null, T.watched), playedSel,
    h('label.check', { class: 'mt3' }, fav, T.onlyFav),
    h('div', { class: 'stack mt4' },
      h('a.btn.ghost', { href: '#/library/' + libId }, T.clear),
      !state.me.profile.is_kids && h('button.btn', { onclick: saveAsSmart }, T.saveSmart)));
  const sortSel = h('select', { 'aria-label': T.sort, onchange: e => { sel.sort = e.target.value; refresh(); } }, f.sorts.map(s => h('option', { value: s.value, selected: s.value === sel.sort }, s.label)));
  const orderSel = h('select', { 'aria-label': 'Irány', onchange: e => { sel.order = e.target.value; refresh(); } }, h('option', { value: 'asc', selected: sel.order === 'asc' }, T.asc), h('option', { value: 'desc', selected: sel.order === 'desc' }, T.desc));
  const qInput = inp('q', 'Szűrés címre…', 'search'); qInput.classList.add('q');
  main.append(h('div.page-head', null, h('div', null, h('h1', null, lib ? lib.name : ''), countEl)),
    h('div.browse', null, filters, h('div', null,
      h('div.toolbar', null, h('button.btn.filters-toggle', { onclick: () => filters.classList.toggle('open') }, icon('filter'), T.filters), qInput, sortSel, orderSel),
      grid, sentinel)));
  const io = new IntersectionObserver(es => { if (es.some(e => e.isIntersecting)) load(false); }, { rootMargin: '600px' });
  io.observe(sentinel);
  onCleanup(() => io.disconnect());
  await load(true);

  function saveAsSmart() {
    const rules = [{ field: 'library', op: 'is', value: String(libId) }];
    sel.genre.forEach(g => rules.push({ field: 'genre', op: 'is', value: g }));
    sel.tag.forEach(t => rules.push({ field: 'tag', op: 'is', value: t }));
    if (sel.year_from) rules.push({ field: 'year', op: 'gte', value: sel.year_from });
    if (sel.year_to) rules.push({ field: 'year', op: 'lte', value: sel.year_to });
    if (sel.min_rating) rules.push({ field: 'community_rating', op: 'gte', value: sel.min_rating });
    if (sel.played) rules.push({ field: 'played', op: 'is', value: sel.played === 'true' });
    if (sel.favorite) rules.push({ field: 'favorite', op: 'is', value: true });
    if (sel.q) rules.push({ field: 'title', op: 'contains', value: sel.q });
    sessionStorage.setItem('kinora-draft-rules', JSON.stringify({ match: 'all', rules, sort: sel.sort, order: sel.order }));
    go('#/collections/new?smart=1');
  }
}

// ===================================================================== item detail
async function viewItem(id) {
  const main = shell('item');
  const d = await GET('/api/items/' + id);
  const it = d.item, ud = it.user_data || {};
  document.title = it.title + ' – ' + T.appName;
  const kidsP = state.me.profile.is_kids;
  const bg = h('div.hero-img');
  const bimg = new Image();
  bimg.onload = () => { bg.style.backgroundImage = `url("${bimg.src}")`; };
  bimg.src = imgURL(it, 'backdrop');
  const playTarget = it.kind === 'movie' || it.kind === 'episode' ? it : d.play_next;
  let playLabel = T.play;
  if (playTarget) {
    const pud = playTarget.user_data || {};
    if (playTarget.kind === 'episode' && playTarget !== it) playLabel = pud.position > 0 ? T.resumeEp(playTarget.season_num, playTarget.episode_num) : T.playEp(playTarget.season_num, playTarget.episode_num);
    else if (pud.position > 0) playLabel = T.resume(fmtTime(pud.position));
  }
  const video = (it.streams || []).find(s => s.type === 'video');
  const audios = (it.streams || []).filter(s => s.type === 'audio');
  const subs = (it.streams || []).filter(s => s.type === 'subtitle');
  const facts = h('div.facts', null,
    it.year && h('span.badge', null, it.year), it.content_rating && h('span.badge', null, it.content_rating),
    it.runtime_sec > 0 && it.kind !== 'series' && h('span.badge', null, fmtRuntime(it.runtime_sec)),
    it.height > 0 && h('span.badge', null, resLabel(it.width, it.height)), it.hdr && h('span.badge.hot', null, it.hdr),
    it.video_codec && h('span.badge', null, it.video_codec.toUpperCase()), it.community_rating && h('span.badge', null, icon('star'), it.community_rating.toFixed(1).replace('.', ',')));

  const playedBtn = h('button.btn.icon' + (ud.played ? '.on' : ''), { 'aria-pressed': ud.played ? 'true' : 'false', title: ud.played ? T.markUnplayed : T.markPlayed, 'aria-label': ud.played ? T.markUnplayed : T.markPlayed, onclick: async () => {
    await POST(`/api/items/${it.id}/played`, { played: !ud.played }); viewItem(id);
  } }, icon('check'));
  const favBtn = h('button.btn.icon' + (ud.favorite ? '.on' : ''), { 'aria-pressed': ud.favorite ? 'true' : 'false', title: T.favorite, 'aria-label': T.favorite, onclick: async () => {
    ud.favorite = !ud.favorite; await POST(`/api/items/${it.id}/favorite`, { favorite: ud.favorite });
    favBtn.classList.toggle('on', ud.favorite); favBtn.setAttribute('aria-pressed', ud.favorite);
  } }, icon('heart', ud.favorite));
  const stars = h('div.stars', { role: 'radiogroup', 'aria-label': 'Saját értékelés' });
  const paintStars = v => stars.querySelectorAll('button').forEach((b, i) => b.classList.toggle('on', v != null && i < v));
  for (let i = 1; i <= 5; i++) stars.append(h('button', { 'aria-label': `${i} csillag`, onclick: async () => {
    const v = ud.stars === i ? null : i; ud.stars = v; await POST(`/api/items/${it.id}/stars`, { stars: v }); paintStars(v);
  } }, icon('star')));
  paintStars(ud.stars);
  const hideBtn = h('button.btn.icon', { title: ud.hidden ? T.unhide : T.hide, 'aria-label': ud.hidden ? T.unhide : T.hide, onclick: async () => {
    await POST(`/api/items/${it.id}/hide`, { hidden: !ud.hidden }); toast(ud.hidden ? T.saved : T.hiddenToast); viewItem(id);
  } }, icon(ud.hidden ? 'eye' : 'eyeOff'));
  const canParty = it.kind === 'movie' || it.kind === 'episode' || !!playTarget;
  const canDl = it.kind === 'movie' || it.kind === 'episode';
  const moreBtn = h('button.btn', { 'aria-haspopup': 'menu', onclick: e => popMenu(e.currentTarget, [
    !kidsP && [T.addToCollection, () => addToCollectionMenu(it, moreBtn)],
    canParty && [T.startParty, async () => { const r = await POST('/api/party', { item_id: (playTarget || it).id, everyone: false }); go(`#/play/${(playTarget || it).id}?party=${r.code}`); }],
    canDl && [T.download + '…', () => modal(T.download, h('div', null, h('p.muted', null, T.dlKept), downloadWidget(it)), close => [h('button.btn', { onclick: close }, T.close)])],
    state.me.user.is_admin && (it.kind === 'movie' || it.kind === 'series') && [T.fixMatch, () => fixMatchDialog(it)],
    state.me.user.is_admin && (it.kind === 'series' || it.kind === 'season') && [T.segAnalyze, async () => { const r = await POST('/api/admin/segments/analyze', { item_id: it.id }); toast(T.segQueued(r.seasons)); }],
  ]) }, icon('more', true), T.moreBtn);
  const tagBox = !kidsP && tagEditor(it);
  const sideInfo = h('dl.side-info', null,
    it.genres && [h('dt', null, T.genres), h('dd', null, it.genres.join(', '))],
    video && [h('dt', null, T.video), h('dd', null, `${video.codec.toUpperCase()} ${it.width}×${it.height}${it.hdr ? ' ' + it.hdr : ''}${it.bitrate ? ', ' + (it.bitrate / 1e6).toFixed(1).replace('.', ',') + ' Mbps' : ''}`)],
    audios.length > 0 && [h('dt', null, T.audio), h('dd', null, audios.map(a => `${(a.lang || T.unknownLang)} ${a.codec}${a.channels ? ' ' + a.channels + 'ch' : ''}`).join(', '))],
    subs.length > 0 && [h('dt', null, T.subtitles), h('dd', null, subs.map(s => (s.lang || T.unknownLang) + (s.forced ? ' (kényszerített)' : '') + (s.text_based ? '' : ' [kép]')).join(', '))],
    it.container && [h('dt', null, T.file), h('dd', null, `${it.container.toUpperCase()}, ${fmtBytes(it.size)}`)],
    d.collections && d.collections.length > 0 && [h('dt', null, T.inCollections), h('dd', null, d.collections.map((c, i) => [i ? ', ' : '', h('a', { href: '#/collections/' + c.id }, c.name)]))]);

  const titleBlock = it.kind === 'episode'
    ? [h('div.sub', null, h('a', { href: '#/item/' + it.series_id }, it.series_title), '  ', T.ep(it.season_num, it.episode_num)), h('h1', null, it.title)]
    : it.kind === 'season' ? [h('div.sub', null, h('a', { href: '#/item/' + it.series_id }, it.series_title)), h('h1', null, it.title)] : [h('h1', null, it.title)];

  main.append(h('section.detail-hero', null, bg, h('div.detail', null,
    h('div.poster', null, artImg(it, it.kind === 'episode' ? 'thumb' : 'poster', it.title)),
    h('div', null, titleBlock, facts, it.overview && h('p.overview', null, it.overview),
      h('div.actions', null,
        playTarget && h('button.btn.primary.big', { onclick: () => go('#/play/' + playTarget.id) }, icon('play'), playLabel),
        playedBtn, favBtn, hideBtn, stars, moreBtn)))));
  main.append(h('div.below', null,
    h('section', null, h('h2', null, T.data), sideInfo),
    tagBox && h('section', null, h('h2', null, T.tags), tagBox)));

  if (it.kind === 'series' && d.seasons) {
    const tabs = h('div.tabs', { role: 'tablist' });
    const list = h('div.episodes');
    const showSeason = async (s, btn) => {
      tabs.querySelectorAll('.chip').forEach(c => { c.classList.remove('on'); c.setAttribute('aria-selected', 'false'); });
      btn.classList.add('on'); btn.setAttribute('aria-selected', 'true');
      const eps = await GET(`/api/items/${s.id}/children`);
      put(list, ...eps.map(episodeRow));
    };
    let first = null;
    d.seasons.forEach(s => {
      const b = h('button.chip', { role: 'tab', onclick: () => showSeason(s, b) }, T.season(s.season_num), s.unplayed_count ? h('span.n', null, s.unplayed_count) : null);
      tabs.append(b);
      if (!first || (d.play_next && d.play_next.parent_id === s.id)) first = [s, b];
    });
    main.append(h('h2', { class: 'mt7' }, T.seasons), tabs, list);
    if (first) showSeason(...first);
  }
  if (it.kind === 'season' && d.episodes) main.append(h('h2', { class: 'mt7 mb3' }, T.episodes), h('div.episodes', null, d.episodes.map(episodeRow)));
  if (it.kind === 'episode' && d.next) main.append(shelf(T.nextEp, [d.next], 'landscape', { playOnClick: true }));
  if (d.similar && d.similar.length) main.append(shelf(T.similar, d.similar, 'poster'));
}

function episodeRow(e) {
  const ud = e.user_data || {};
  const art = artImg(e, 'thumb', e.title);
  if (ud.position > 0 && e.runtime_sec) art.append(h('div.prog', null, h('i', { style: { width: Math.min(100, ud.position / e.runtime_sec * 100) + '%' } })));
  return h('div.ep.card' + (ud.played ? '.watched' : ''), { tabindex: 0, role: 'link', onclick: ev => { if (!ev.target.closest('button')) go('#/play/' + e.id); }, onkeydown: ev => { if (ev.key === 'Enter' && ev.target === ev.currentTarget) go('#/play/' + e.id); } },
    art,
    h('div', null, h('h3', null, e.episode_num != null ? `${e.episode_num}. ` : '', e.title), h('p', null, e.overview || ''), h('span.muted.small', null, e.runtime_sec ? fmtRuntime(e.runtime_sec) : '')),
    h('div.row-gap', null,
      h('button.btn.icon' + (ud.played ? '.on' : ''), { title: ud.played ? T.markUnplayed : T.markPlayed, 'aria-label': ud.played ? T.markUnplayed : T.markPlayed, onclick: async ev => {
        const btn = ev.currentTarget;
        await POST(`/api/items/${e.id}/played`, { played: !ud.played }); ud.played = !ud.played;
        btn.classList.toggle('on', ud.played); btn.closest('.ep').classList.toggle('watched', ud.played);
      } }, icon('check'))));
}

async function addToCollectionMenu(it, anchor) {
  const cols = (await GET('/api/collections')).filter(c => c.kind === 'manual');
  const target = it.kind === 'season' || it.kind === 'episode' ? it.series_id || it.id : it.id;
  popMenu(anchor, [
    ...cols.map(c => [c.name, async () => { await POST(`/api/collections/${c.id}/items/${target}`); toast(T.addedTo(c.name)); }]),
    [T.newCollection, () => {
      const name = h('input', { type: 'text', maxlength: 80 });
      const shared = h('input', { type: 'checkbox' });
      modal(T.newManual, h('div', null, field(T.collName, name), h('label.check', null, shared, T.shared)), close => [h('button.btn', { onclick: close }, T.cancel),
        h('button.btn.primary', { onclick: async () => {
          const c = await POST('/api/collections', { name: name.value, kind: 'manual', shared: shared.checked });
          await POST(`/api/collections/${c.id}/items/${target}`); close(); toast(T.addedTo(c.name));
        } }, T.create)]);
    }],
  ]);
}

function tagEditor(it) {
  let tags = [...(it.tags || [])];
  const box = h('div.tags', { 'aria-label': T.tags });
  const listId = 'tagopts' + it.id;
  const dl = h('datalist', { id: listId });
  GET('/api/facets').then(f => put(dl, ...f.tags.map(t => h('option', { value: t.value })))).catch(() => {});
  const save = async () => { const r = await api('PUT', `/api/items/${it.id}/tags`, { tags }); tags = r.tags; render(); };
  const input = h('input', { type: 'text', placeholder: T.addTag, list: listId, 'aria-label': T.addTag, onkeydown: e => {
    if (e.key === 'Enter' && input.value.trim()) { e.preventDefault(); tags.push(input.value.trim()); input.value = ''; save(); }
  } });
  const render = () => put(box, icon('tag'), ...tags.map(t => h('span.chip', null, t, h('button', { class: 'chip-x', 'aria-label': `${t} törlése`, onclick: () => { tags = tags.filter(x => x !== t); save(); } }, icon('x')))), input, dl);
  render();
  return h('div', { class: 'mb3' }, box);
}

function downloadWidget(it) {
  const q = h('select', { 'aria-label': T.downloadQ, style: { width: 'auto' } },
    h('option', { value: 'original' }, T.original), ['1080p', '720p', '480p', '360p'].map(v => h('option', { value: v }, v)));
  const btn = h('button.btn', { onclick: async () => {
    const r = await POST(`/api/items/${it.id}/download`, { quality: q.value });
    if (r.url) { location.href = r.url; return; }
    toast(T.dlStarted);
  } }, icon('dl'), T.download);
  return h('span.row-gap', null, q, btn);
}

async function fixMatchDialog(it) {
  const qi = h('input', { type: 'text', value: it.title });
  const yi = h('input', { type: 'number', value: it.year || '' });
  const idi = h('input', { type: 'number', placeholder: 'pl. 603' });
  const results = h('div', { class: 'stack mt3' });
  const kind = it.kind === 'series' ? 'series' : 'movie';
  let m;
  const apply = async tmdbId => { await POST(`/api/items/${it.id}/metadata/match`, { tmdb_id: +tmdbId }); m.close(); toast(T.saved); viewItem(it.id); };
  const search = async () => {
    const r = await GET(`/api/admin/metadata/search?kind=${kind}&q=${encodeURIComponent(qi.value)}&year=${yi.value || 0}`);
    put(results, ...r.slice(0, 10).map(x => h('div.entry', null,
      x.poster ? h('img', { src: x.poster, alt: '', class: 'match-thumb' }) : null,
      h('div', { class: 'grow-min' }, h('b', null, x.title), ' ', h('span.muted', null, x.year || ''), h('div.small.muted', null, (x.overview || '').slice(0, 140))),
      h('button.btn', { onclick: () => apply(x.id) }, T.matchApply))));
    if (!r.length) results.append(h('p.muted', null, T.searchEmpty));
  };
  m = modal(T.fixMatch, h('div', null, h('div.grid2', null, field('Cím', qi), field('Év', yi)), h('button.btn', { onclick: search }, T.matchSearch),
    results, h('div', { class: 'mt4' }, field(T.tmdbId, idi))), close => [h('button.btn', { onclick: close }, T.cancel), h('button.btn.primary', { onclick: () => idi.value && apply(idi.value) }, T.matchApply)]);
}

// ===================================================================== collections
async function viewCollections() {
  const main = shell('collections');
  const cols = await GET('/api/collections');
  const kids = state.me.profile.is_kids;
  main.append(h('div.page-head', null, h('h1', null, T.collections), !kids && h('div.row-gap', null,
    h('button.btn', { onclick: newManual }, icon('plus'), T.newManual), h('a.btn.primary', { href: '#/collections/new?smart=1' }, icon('plus'), T.newSmart))));
  if (!cols.length) { main.append(h('p.empty', null, T.collEmpty)); return; }
  for (const c of cols) {
    const r = await GET(`/api/collections/${c.id}?limit=20`);
    const head = [c.name, c.kind === 'smart' ? ' · smart' : '', c.shared ? ' · közös' : ''].join('');
    main.append(shelf(head, r.items, 'poster', { more: '#/collections/' + c.id }));
  }
  function newManual() {
    const name = h('input', { type: 'text', maxlength: 80 });
    const shared = h('input', { type: 'checkbox' });
    const pin = h('input', { type: 'checkbox' });
    modal(T.newManual, h('div', null, field(T.collName, name), h('label.check', null, shared, T.shared), h('label.check', null, pin, T.pinned)), close => [h('button.btn', { onclick: close }, T.cancel),
      h('button.btn.primary', { onclick: async () => { const c = await POST('/api/collections', { name: name.value, kind: 'manual', shared: shared.checked, pinned: pin.checked }); close(); go('#/collections/' + c.id); } }, T.create)]);
  }
}

async function viewCollection(id) {
  const main = shell('collections');
  const r = await GET(`/api/collections/${id}?limit=500`);
  const c = r.collection, kids = state.me.profile.is_kids;
  const pinBtn = !kids && h('button.btn' + (c.pinned ? '.on' : ''), { onclick: async () => { await api('PATCH', '/api/collections/' + id, { pinned: !c.pinned }); viewCollection(id); } }, c.pinned ? 'Kitűzve a kezdőlapra' : T.pinned);
  main.append(h('div.page-head', null, h('div', null, h('h1', null, c.name), h('span.muted', null, `${r.total} cím · ${c.kind === 'smart' ? T.smart : T.manual}${c.shared ? ' · közös' : ''}`)),
    !kids && h('div.row-gap', null, pinBtn,
      c.kind === 'smart' && h('a.btn', { href: `#/collections/${id}/edit` }, T.edit),
      c.kind === 'manual' && h('button.btn', { onclick: () => renameColl(c) }, T.edit),
      h('button.btn.danger', { onclick: async () => { if (await confirmBox(T.confirmDelete(c.name))) { await api('DELETE', '/api/collections/' + id); go('#/collections'); } } }, T.delete))));
  const grid = h('div.grid');
  const items = r.items;
  const render = () => put(grid, ...items.map((it, i) => card(it, 'poster', { menu: kids || c.kind !== 'manual' ? null : (x, btn) => popMenu(btn, [
    i > 0 && [T.moveUp, () => move(i, -1)], i < items.length - 1 && [T.moveDown, () => move(i, 1)],
    [T.removeFromColl, async () => { await api('DELETE', `/api/collections/${id}/items/${x.id}`); items.splice(i, 1); render(); toast(T.removedFrom(c.name)); }]]) })));
  const move = async (i, d) => { [items[i], items[i + d]] = [items[i + d], items[i]]; render(); await api('PUT', `/api/collections/${id}/order`, { items: items.map(x => x.id) }); };
  render();
  main.append(items.length ? grid : h('p.empty', null, T.noResults));
  function renameColl(c) {
    const name = h('input', { type: 'text', value: c.name });
    const shared = h('input', { type: 'checkbox', checked: c.shared });
    modal(T.edit, h('div', null, field(T.collName, name), h('label.check', null, shared, T.shared)), close => [h('button.btn', { onclick: close }, T.cancel),
      h('button.btn.primary', { onclick: async () => { await api('PATCH', '/api/collections/' + c.id, { name: name.value, shared: shared.checked }); close(); viewCollection(c.id); } }, T.save)]);
  }
}

async function viewCollectionEdit(id, q) {
  const main = shell('collections');
  const f = state.facets || (state.facets = await facets());
  const libs = state.libs || (state.libs = await GET('/api/libraries'));
  const allCols = await GET('/api/collections');
  let c = { name: '', kind: 'smart', shared: false, pinned: false, rules: { match: 'all', rules: [], sort: 'title', order: 'asc', limit: 0 } };
  if (id) c = (await GET(`/api/collections/${id}?limit=1`)).collection;
  else { const draft = sessionStorage.getItem('kinora-draft-rules'); if (draft) { c.rules = JSON.parse(draft); sessionStorage.removeItem('kinora-draft-rules'); } }
  const rs = c.rules || { match: 'all', rules: [] };
  const name = h('input', { type: 'text', value: c.name, maxlength: 80 });
  const shared = h('input', { type: 'checkbox', checked: c.shared });
  const pinned = h('input', { type: 'checkbox', checked: c.pinned });
  const match = h('select', null, h('option', { value: 'all', selected: rs.match !== 'any' }, T.matchAll), h('option', { value: 'any', selected: rs.match === 'any' }, T.matchAny));
  const sort = h('select', null, f.sorts.map(s => h('option', { value: s.value, selected: s.value === (rs.sort || 'title') }, s.label)));
  const order = h('select', null, h('option', { value: 'asc', selected: rs.order !== 'desc' }, T.asc), h('option', { value: 'desc', selected: rs.order === 'desc' }, T.desc));
  const limit = h('input', { type: 'number', min: 0, value: rs.limit || '' });
  const rows = h('div', { class: 'stack' });
  const previewHead = h('h2', null, T.preview(0));
  const previewGrid = h('div.grid');
  const fieldDesc = name => f.fields.find(x => x.field === name);

  const valueInput = (fd, v) => {
    const opt = (val, label) => h('option', { value: val, selected: String(v) === String(val) }, label);
    switch (fd.type) {
      case 'enum': return h('select', null, fd.enum.map(o => opt(o.value, o.label)));
      case 'library': return h('select', null, libs.map(l => opt(l.id, l.name)));
      case 'genre': return h('select', null, f.genres.map(g => opt(g.value, g.value)));
      case 'tag': return h('select', null, f.tags.map(t => opt(t.value, t.value)));
      case 'collection': return h('select', null, allCols.filter(x => x.id !== id).map(x => opt(x.id, x.name)));
      case 'bool': return h('select', null, opt('true', T.yes), opt('false', T.no));
      case 'number': case 'days': return h('input', { type: 'number', value: v ?? '' });
      default: return h('input', { type: 'text', value: v ?? '' });
    }
  };
  const ruleRow = r => {
    const fsel = h('select', { 'aria-label': 'Mező' }, f.fields.map(x => h('option', { value: x.field, selected: x.field === r.field }, x.label)));
    const osel = h('select', { 'aria-label': 'Művelet' });
    let vin;
    const row = h('div.row-gap.rule-row');
    const rebuild = (keepV) => {
      const fd = fieldDesc(fsel.value);
      put(osel, ...fd.ops.map(o => h('option', { value: o, selected: o === r.op }, T.ops[o] || o)));
      const nv = valueInput(fd, keepV ? r.value : undefined);
      nv.setAttribute('aria-label', 'Érték');
      if (vin) vin.replaceWith(nv); vin = nv;
      for (const el of [osel, vin]) { el.onchange = preview; el.oninput = preview; }
    };
    fsel.onchange = () => { r.op = null; rebuild(false); preview(); };
    rebuild(true);
    row.append(fsel, osel, vin, h('button.btn.icon', { 'aria-label': T.delete, onclick: () => { row.remove(); preview(); } }, icon('x')));
    row._get = () => ({ field: fsel.value, op: osel.value, value: row.children[2].value });
    return row;
  };
  for (const r of rs.rules || []) rows.append(ruleRow(r));
  const collect = () => ({ match: match.value, rules: [...rows.children].map(r => r._get()), sort: sort.value, order: order.value, limit: +limit.value || 0 });
  const preview = debounce(async () => {
    try {
      const r = await POST('/api/collections/preview', collect(), { quiet: true });
      previewHead.textContent = T.preview(r.total);
      put(previewGrid, ...r.items.map(it => card(it, 'poster')));
    } catch (e) { previewHead.textContent = e.message; put(previewGrid); }
  }, 300);
  for (const el of [match, sort, order, limit]) { el.onchange = preview; el.oninput = preview; }
  main.append(h('div.page-head', null, h('h1', null, id ? c.name : T.newSmart)),
    h('div.panel', null,
      h('div.grid2', null, field(T.collName, name), h('div', { class: 'pt5' }, h('label.check', null, shared, T.shared), h('label.check', null, pinned, T.pinned))),
      h('h2', null, T.rules), field('', match), rows,
      h('button.btn', { class: 'mt3', onclick: () => { rows.append(ruleRow({ field: 'genre', op: 'is' })); preview(); } }, icon('plus'), T.addRule),
      h('div.grid2', { class: 'mt4' }, field(T.sort, h('div.row-gap', { class: 'nowrap' }, sort, order)), field(T.limit, limit)),
      h('div.row-gap', { class: 'end' }, h('a.btn', { href: id ? '#/collections/' + id : '#/collections' }, T.cancel),
        h('button.btn.primary', { onclick: async () => {
          const body = { name: name.value, kind: 'smart', rules: collect(), shared: shared.checked, pinned: pinned.checked };
          const r = id ? await api('PATCH', '/api/collections/' + id, body) : await POST('/api/collections', body);
          toast(T.saved); go('#/collections/' + (id || r.id));
        } }, T.save))),
    previewHead, h('div', { class: 'mt3' }, previewGrid));
  preview();
}

// ===================================================================== downloads
async function viewDownloads() {
  const main = shell('downloads');
  main.append(h('div.page-head', null, h('div', null, h('h1', null, T.downloads), h('span.muted', null, T.dlKept))));
  const box = h('div');
  main.append(box);
  let timer;
  const render = async () => {
    const r = await GET('/api/downloads');
    if (!r.jobs.length) { put(box, h('p.empty', null, T.dlEmpty)); return; }
    put(box, h('div.scroll-x', null, h('table.tbl', null, h('thead', null, h('tr', null, h('th', null, 'Cím'), h('th', null, T.quality), h('th', null, 'Állapot'), h('th', null, ''))),
      h('tbody', null, r.jobs.map(j => h('tr', null,
        h('td', null, h('a', { href: '#/item/' + j.item_id }, j.file_name)),
        h('td', null, j.quality),
        h('td', { class: 'td-progress' }, j.status === 'running' ? h('div', null, h('div.bar', null, h('i', { style: { width: Math.round(j.progress * 100) + '%' } })), h('span.small.muted', null, Math.round(j.progress * 100) + '%')) : h('span' + (j.status === 'error' ? '.bad' : ''), { title: j.error || '' }, T.dlStatus[j.status] + (j.error ? ': ' + j.error : '')), j.size ? h('div.small.muted', null, fmtBytes(j.size)) : null),
        h('td', null, j.status === 'done' && h('a.btn.primary', { href: `/api/downloads/${j.key}/file`, download: j.file_name }, icon('dl'), T.saveFile))))))));
    if (r.jobs.some(j => j.status === 'running' || j.status === 'queued')) timer = setTimeout(render, 2000);
  };
  onCleanup(() => clearTimeout(timer));
  await render();
}

// ===================================================================== account
async function viewAccount() {
  const main = shell('account');
  const cur = h('input', { type: 'password', autocomplete: 'current-password' });
  const nw = h('input', { type: 'password', autocomplete: 'new-password' });
  main.append(h('div.page-head', null, h('h1', null, T.account)),
    h('div.panel', { class: 'narrow' }, h('h2', null, 'Jelszócsere'), field('Jelenlegi jelszó', cur), field('Új jelszó', nw),
      h('button.btn.primary', { onclick: async () => { await POST('/api/me/password', { current: cur.value, new: nw.value }); cur.value = nw.value = ''; toast(T.saved); } }, T.save)),
    devicesPanel(),
    h('p.muted', null, `${state.me.user.username} · ${state.me.server_name} · ${T.version} ${state.me.version}`));
}

function deviceRow(d, onChange, showUser) {
  return h('div.entry', null, h('div', null,
    h('div', null, h('strong', null, d.name || T.platforms[d.platform] || d.platform), d.current && h('span.muted', null, ' · ' + T.thisDevice)),
    h('p.muted.small', null, [showUser && d.username, T.platforms[d.platform] || d.platform, d.app_version && 'v' + d.app_version, d.ip,
      T.lastActive + ': ' + T.ago(Date.now() / 1000 - d.last_seen)].filter(Boolean).join(' · ')),
    !d.current && h('button.btn.danger', { onclick: async () => { await api('DELETE', '/api/devices/' + d.id); toast(T.deleted); onChange(); } }, T.signOutDevice)));
}

function devicesPanel() {
  const box = h('div.panel', { class: 'narrow' });
  const load = async () => {
    const r = await GET('/api/devices');
    put(box, h('h2', null, T.myDevices), r.devices.map(d => deviceRow(d, load)),
      r.devices.length > 1 && h('button.btn', { onclick: async () => { const x = await api('DELETE', '/api/devices'); toast(T.signedOutN(x.removed)); load(); } }, T.signOutOthers));
  };
  load();
  return box;
}

// ===================================================================== quick connect (approve a TV / phone)
async function viewQuickConnect(code) {
  const main = shell('account');
  const me = state.me;
  if (me.profile.is_kids || me.profile_locked) { main.append(h('div.center', { class: 'tall' }, h('p.lede', null, T.qcKids))); return; }
  const box = h('div.auth');
  main.append(h('div.center', { class: 'tall' }, box));
  const inp = h('input.pin-input', { type: 'text', inputmode: 'numeric', maxlength: 7, autocomplete: 'off', 'aria-label': T.qcCode, value: code });
  const ask = () => { put(box, h('form', { onsubmit: async e => { e.preventDefault(); await lookup(inp.value); } },
    h('h1', null, T.connectDevice), h('p.lede', null, T.qcLede), inp, h('button.btn.primary.big', { type: 'submit', class: 'wide mt3' }, T.next))); inp.focus(); };
  async function lookup(c) {
    let info;
    try { info = await POST('/api/quickconnect/lookup', { code: c }); } catch { return ask(); }
    const radios = [[0, T.qcAnyProfile, null], ...me.profiles.map(p => [p.id, p.name, p])].map(([id, label, p]) =>
      h('label.check', null, h('input', { type: 'radio', name: 'qcp', value: id, checked: p ? p.id === me.profile.id : false }), ' ', label,
        p && p.is_kids ? ' (' + T.kids + ')' : '', p && p.has_pin ? ' – PIN' : ''));
    put(box, h('h1', null, T.connectDevice), h('p.lede', null, T.qcWants(info.device_name, T.platforms[info.platform] || info.platform)),
      h('div.code', null, info.code),
      h('fieldset', { class: 'mt3' }, h('legend', null, T.qcProfile), radios),
      h('p.muted.small', null, T.qcPinNote, ' ', T.qcKidsNote),
      h('div.row-gap', { class: 'mt3' },
        h('button.btn.primary.big', { onclick: async () => {
          const pid = +(box.querySelector('input[name=qcp]:checked')?.value || 0);
          await POST('/api/quickconnect/approve', { code: info.code, profile_id: pid });
          put(box, h('h1', null, T.connectDevice), h('p.lede', null, T.qcApproved), h('a.btn', { href: '#/home' }, T.home));
        } }, T.approve),
        h('button.btn', { onclick: async () => { await POST('/api/quickconnect/approve', { code: info.code, action: 'deny' }); toast(T.qcDenied); ask(); } }, T.deny)));
    box.querySelector('button.primary')?.focus();
  }
  if (code && code.replace(/[^0-9]/g, '').length === 6) await lookup(code); else ask();
}

// ===================================================================== party join
async function viewPartyJoin(code) {
  if (code) {
    try { const info = await GET('/api/party/' + encodeURIComponent(code), { quiet: true }); return go(`#/play/${info.item_id}?party=${info.code}`); }
    catch (e) { if (e.status !== 401 && e.status !== 428) toast(e.message || T.partyNotFound, true); }
  }
  const main = shell('party');
  const inp = h('input.pin-input', { type: 'text', maxlength: 6, autocomplete: 'off', 'aria-label': T.partyCodeLabel, class: 'upper' });
  main.append(h('div.center', { class: 'tall' }, h('form.auth', { onsubmit: e => { e.preventDefault(); go('#/party/' + inp.value.trim().toUpperCase()); } },
    h('h1', null, T.partyJoinTitle), h('p.lede', null, T.partyCodeLabel), inp, h('button.btn.primary.big', { type: 'submit', class: 'wide mt3' }, T.enter))));
  inp.focus();
}

// ===================================================================== player
function detectCaps() {
  const v = document.createElement('video');
  const ms = window.MediaSource || window.ManagedMediaSource;
  const can = t => (ms && ms.isTypeSupported && ms.isTypeSupported(t)) || v.canPlayType(t) === 'probably';
  const caps = { video: [], audio: [], containers: [], hdr: false };
  if (can('video/mp4; codecs="avc1.640028"')) caps.video.push('h264');
  if (can('video/mp4; codecs="hvc1.1.6.L120.90"') || can('video/mp4; codecs="hev1.1.6.L120.90"')) caps.video.push('hevc');
  if (can('video/webm; codecs="vp9"') || can('video/mp4; codecs="vp09.00.10.08"')) caps.video.push('vp9');
  if (can('video/mp4; codecs="av01.0.05M.08"')) caps.video.push('av1');
  if (can('audio/mp4; codecs="mp4a.40.2"')) caps.audio.push('aac');
  if (can('audio/mpeg')) caps.audio.push('mp3');
  if (can('audio/webm; codecs="opus"') || can('audio/mp4; codecs="opus"')) caps.audio.push('opus');
  if (can('audio/flac') || can('audio/mp4; codecs="flac"')) caps.audio.push('flac');
  if (can('audio/mp4; codecs="ac-3"')) caps.audio.push('ac3');
  if (can('audio/mp4; codecs="ec-3"')) caps.audio.push('eac3');
  if (v.canPlayType('video/mp4')) caps.containers.push('mp4');
  if (v.canPlayType('video/webm')) caps.containers.push('webm');
  if (v.canPlayType('video/quicktime')) caps.containers.push('mov');
  if (v.canPlayType('video/x-matroska') === 'probably') caps.containers.push('mkv');
  caps.hdr = !!(window.matchMedia && matchMedia('(dynamic-range: high)').matches && caps.video.includes('hevc'));
  return caps;
}
const langName = l => ({ hun: 'Magyar', eng: 'Angol', ger: 'Német', fre: 'Francia', spa: 'Spanyol', ita: 'Olasz', jpn: 'Japán', rus: 'Orosz', pol: 'Lengyel', cze: 'Cseh', slo: 'Szlovák', rum: 'Román', kor: 'Koreai', chi: 'Kínai' }[l] || (l ? l : T.unknownLang));

async function viewPlayer(id, q) {
  const profile = state.me.profile;
  const root = app();
  put(root);
  const video = h('video', { playsinline: true, preload: 'auto', 'aria-label': 'Videó' });
  const spinner = h('div.spin', { role: 'progressbar', 'aria-label': T.loading });
  const mid = h('div.p-mid', null, spinner);
  const titleEl = h('h2'), subEl = h('div.s');
  const timeEl = h('span.p-time', null, '0:00 / 0:00');
  const rail = h('div.rail'), ready = h('div.ready'), done = h('div.done'), knob = h('div.knob'), hoverT = h('div.hover-t.hidden');
  const timeline = h('div.timeline', { role: 'slider', tabindex: 0, 'aria-label': 'Idővonal', 'aria-valuemin': 0 }, rail, ready, done, knob, hoverT);
  const playBtn = h('button.p-btn', { 'aria-label': T.play }, icon('play'));
  const volBtn = h('button.p-btn', { 'aria-label': T.mute }, icon('vol'));
  const vol = h('input.vol', { type: 'range', min: 0, max: 1, step: 0.05, value: localStorage.getItem('kinora-vol') || 1, 'aria-label': 'Hangerő' });
  const nextBtn = h('button.p-btn.hidden', { 'aria-label': T.nextEp, title: T.nextEp + ' (N)' }, icon('next'));
  const ccBtn = h('button.p-btn', { 'aria-label': T.subtitle + ' / ' + T.audioTrack, title: T.subtitle + ' (C)' }, icon('cc'));
  const gearBtn = h('button.p-btn', { 'aria-label': T.quality }, icon('gear'));
  const whyBtn = h('button.p-btn', { 'aria-label': T.why, title: T.why }, icon('info'));
  const favBtn = h('button.p-btn', { 'aria-label': T.favorite, title: T.favorite }, icon('heart'));
  const moreBtn = h('button.p-btn', { 'aria-label': T.moreActions, title: T.moreActions }, icon('more', true));
  const fsBtn = h('button.p-btn', { 'aria-label': T.fullscreen, title: T.fullscreen + ' (F)' }, icon('fs'));
  const pop = h('div.p-pop.hidden');
  const upnext = h('div.upnext.hidden');
  const skipBtn = h('button.btn.primary.big.skip-seg.hidden', { onclick: () => doSkip() });
  const ui = h('div.ui', null,
    h('div.p-top', null, h('button.p-btn', { 'aria-label': T.back, onclick: () => exit() }, icon('back')), h('div.t', null, titleEl, subEl)),
    mid,
    h('div.p-bottom', null, timeline, h('div.p-bar', null,
      playBtn, h('button.p-btn', { 'aria-label': T.rewind, title: T.rewind, onclick: () => seekBy(-10) }, icon('rw')), h('button.p-btn', { 'aria-label': T.forward, title: T.forward, onclick: () => seekBy(10) }, icon('ff')),
      volBtn, vol, timeEl, h('span.grow'), nextBtn, favBtn, moreBtn, whyBtn, ccBtn, gearBtn, fsBtn)));
  const stage = h('div.stage', null, video, ui, pop, upnext, skipBtn);
  const player = h('div.player', null, stage);
  root.append(player);

  // ---------------------------------------------------------------- state
  let info = null, item = null, hls = null, offset = 0, mode = 'direct', session = null;
  let audioIdx = null, burnIdx = null, maxBitrate = 0, force = '', subIdx = -1, showAllLangs = false;
  let duration = 0, credits = null, nextItem = null, nextShown = false, nextTimer = null, lastReport = 0, exiting = false;
  let dragging = false, dead = false;
  let segments = [], shownSeg = null, markStart = null;
  const skipped = new Set();
  const caps = detectCaps();
  const partyCode = q.get('party');
  let party = null;
  video.volume = +vol.value;

  const pos = () => offset + (video.currentTime || 0);
  const isHLS = () => mode !== 'direct';
  const transcodedEnd = () => {
    if (!isHLS()) return duration;
    let end = 0;
    if (hls && hls.levels && hls.levels[hls.currentLevel >= 0 ? hls.currentLevel : 0]?.details) end = hls.levels[hls.currentLevel >= 0 ? hls.currentLevel : 0].details.totalduration;
    else if (video.seekable && video.seekable.length) end = video.seekable.end(video.seekable.length - 1);
    return offset + end;
  };

  async function start(startAt, opts = {}) {
    if (dead) return;
    clearUpNext();
    put(mid, spinner);
    const body = { caps, start: startAt, max_bitrate: maxBitrate, force, prev_session: session || '' };
    if (audioIdx != null) body.audio_idx = audioIdx;
    if (burnIdx != null) body.burn_sub_idx = burnIdx;
    let r;
    try { r = await POST(`/api/items/${id}/playback`, body, { quiet: true }); }
    catch (e) { return showError(e.message, e.body && e.body.reasons); }
    if (dead) { if (r.session) fetch('/api/hls/' + r.session, { method: 'DELETE', credentials: 'same-origin' }); return; }
    info = r; mode = r.mode; session = r.session || null; offset = r.start_offset || 0;
    duration = r.duration || 0; credits = r.credits_start ?? null;
    segments = r.segments || [];
    const cr = segments.find(g => g.type === 'credits');
    if (cr) credits = cr.start;
    if (audioIdx == null) audioIdx = r.audio_idx;
    if (hls) { hls.destroy(); hls = null; }
    video.removeAttribute('src');
    const autoplay = opts.paused ? false : true;
    if (mode === 'direct') {
      video.src = r.url;
      video.addEventListener('loadedmetadata', () => { if (startAt > 0) video.currentTime = startAt; }, { once: true });
    } else if (window.Hls && Hls.isSupported()) {
      hls = new Hls({ maxBufferLength: 30, backBufferLength: 60, startPosition: 0, manifestLoadingMaxRetry: 6, levelLoadingMaxRetry: 6, fragLoadingMaxRetry: 6 });
      hls.on(Hls.Events.ERROR, (_, data) => {
        if (!data.fatal) return;
        if (data.type === Hls.ErrorTypes.MEDIA_ERROR) { hls.recoverMediaError(); return; }
        showError(T.playErr + ': ' + (data.details || data.type));
      });
      hls.on(Hls.Events.LEVEL_UPDATED, paintTimeline);
      hls.loadSource(r.url);
      hls.attachMedia(video);
    } else if (video.canPlayType('application/vnd.apple.mpegurl')) {
      video.src = r.url;
    } else {
      return showError('A böngésző nem támogatja a HLS-t.');
    }
    if (autoplay) video.play().catch(() => { playBtn.focus(); });
    applySubtitle();
    paintTimeline();
  }

  async function seekTo(t) {
    t = Math.max(0, Math.min(duration ? duration - 1 : t, t));
    if (!isHLS()) { video.currentTime = t; return; }
    if (t >= offset && t <= transcodedEnd() - 1) { video.currentTime = t - offset; return; }
    await start(t, { paused: video.paused && !party });
  }
  function seekBy(d) { userSeek(pos() + d); }
  function userSeek(t) { if (party) party.send({ t: 'seek', pos: t }); seekTo(t); }
  function togglePlay() {
    if (video.paused) { if (party) party.send({ t: 'play' }); video.play().catch(() => {}); }
    else { if (party) party.send({ t: 'pause', pos: pos() }); video.pause(); }
  }

  function showError(msg, reasons) {
    put(mid, h('div.p-error', { role: 'alert' }, h('h2', null, T.playErr), h('p', null, msg),
      reasons && reasons.length && h('ul.why-list.muted', null, reasons.map(r => h('li', null, r))),
      h('div.row-gap', null, h('button.btn.primary', { onclick: () => start(pos()) }, T.retry),
        force !== 'transcode' && h('button.btn', { onclick: () => { force = 'transcode'; start(pos()); } }, T.forceTranscode),
        h('button.btn', { onclick: () => exit() }, T.back))));
  }

  // ---------------------------------------------------------------- subtitles (fetched VTT, shifted by the HLS start offset)
  let subTrack = null, subGen = 0;
  // cues are only readable while the track is not 'disabled', so empty it first
  const clearCues = () => { if (subTrack) { subTrack.mode = 'hidden'; for (const c of [...(subTrack.cues || [])]) subTrack.removeCue(c); subTrack.mode = 'disabled'; } };
  async function applySubtitle() {
    const my = ++subGen; // a newer call wins; stale fetches are dropped
    clearCues();
    if (subIdx < 0 || !info) return;
    const st = info.streams.find(s => s.type === 'subtitle' && s.type_index === subIdx);
    if (!st || !st.text_based) return;
    try {
      const res = await fetch(`/api/items/${id}/subtitles/${subIdx}`, { credentials: 'same-origin' });
      if (!res.ok) throw new Error((await res.json().catch(() => ({}))).error || res.status);
      const cues = parseVTT(await res.text());
      if (my !== subGen) return;
      clearCues();
      if (!subTrack) subTrack = video.addTextTrack('subtitles', 'Kinora', st.lang || 'und');
      for (const c of cues) {
        const s = c.start - offset, e = c.end - offset;
        if (e <= 0) continue;
        try { const cue = new VTTCue(Math.max(0, s), e, c.text); cue.line = -3; subTrack.addCue(cue); } catch { /* ignore */ }
      }
      subTrack.mode = 'showing';
    } catch (e) { toast('Felirat: ' + e.message, true); }
  }
  function parseVTT(text) {
    const out = [];
    const ts = s => { const p = s.trim().split(':').map(Number); return p.length === 3 ? p[0] * 3600 + p[1] * 60 + p[2] : p[0] * 60 + p[1]; };
    for (const block of text.replace(/\r/g, '').split(/\n\n+/)) {
      const lines = block.split('\n');
      const i = lines.findIndex(l => l.includes('-->'));
      if (i < 0) continue;
      const [a, b] = lines[i].split('-->');
      out.push({ start: ts(a), end: ts(b.trim().split(' ')[0]), text: lines.slice(i + 1).join('\n').replace(/<[^>]+>/g, '') });
    }
    return out;
  }
  function chooseInitialSubtitle() {
    const langs = (profile.sub_langs || '').split(',').filter(Boolean);
    const subs = info.streams.filter(s => s.type === 'subtitle' && s.text_based);
    if (profile.sub_mode === 'off' || !subs.length) return;
    const audio = info.streams.find(s => s.type === 'audio' && s.type_index === audioIdx);
    const forced = subs.find(s => s.forced && langs.includes(s.lang));
    const audioForeign = !audio || !langs.length || audio.lang !== langs[0];
    if (profile.sub_mode === 'always' || audioForeign) {
      for (const l of langs) { const s = subs.find(x => x.lang === l && !x.forced); if (s) { subIdx = s.type_index; return; } }
    }
    if (forced) subIdx = forced.type_index;
  }

  // ---------------------------------------------------------------- popovers
  function openPop(build) {
    if (!pop.classList.contains('hidden') && pop._b === build) { pop.classList.add('hidden'); return; }
    pop._b = build; put(pop, build()); pop.classList.remove('hidden');
    pop.querySelector('button')?.focus();
  }
  const opt = (label, on, fn) => h('button.opt' + (on ? '.on' : ''), { 'aria-pressed': on ? 'true' : 'false', onclick: () => { pop.classList.add('hidden'); fn(); } }, on ? icon('check') : h('span', { class: 'opt-gap' }), label);
  function buildTracks() {
    const langs = (profile.sub_langs || '').split(',').filter(Boolean), alangs = (profile.audio_langs || '').split(',').filter(Boolean);
    const audios = info.streams.filter(s => s.type === 'audio');
    const subs = info.streams.filter(s => s.type === 'subtitle');
    const visA = showAllLangs || !alangs.length ? audios : audios.filter(a => alangs.includes(a.lang) || a.type_index === audioIdx);
    const visS = showAllLangs || !langs.length ? subs : subs.filter(s => langs.includes(s.lang) || s.type_index === subIdx || s.type_index === burnIdx);
    const hiddenCount = (audios.length - visA.length) + (subs.length - visS.length);
    return [
      h('h3', null, T.audioTrack),
      ...visA.map(a => opt(`${langName(a.lang)} – ${a.codec}${a.channels ? ' ' + a.channels + 'ch' : ''}${a.title ? ' (' + a.title + ')' : ''}`, a.type_index === audioIdx, () => { audioIdx = a.type_index; start(pos()); })),
      h('h3', null, T.subtitle),
      opt(T.subOff, subIdx < 0 && burnIdx == null, () => { subIdx = -1; if (burnIdx != null) { burnIdx = null; start(pos()); } else applySubtitle(); }),
      ...visS.map(s => s.text_based
        ? opt(`${langName(s.lang)}${s.forced ? ' (kényszerített)' : ''}${s.external ? ' · külső' : ''}`, s.type_index === subIdx, () => { subIdx = s.type_index; if (burnIdx != null) { burnIdx = null; start(pos()); } else applySubtitle(); })
        : opt(`${langName(s.lang)} · ${T.burnIn} (${s.codec})`, s.type_index === burnIdx, () => { subIdx = -1; burnIdx = s.type_index; applySubtitle(); start(pos()); })),
      (hiddenCount > 0 || showAllLangs) && h('label.check', { class: 'mt3' }, h('input', { type: 'checkbox', checked: showAllLangs, onchange: e => { showAllLangs = e.target.checked; pop._b = null; openPop(buildTracks); } }), T.showAll),
    ];
  }
  function buildQuality() {
    const q = [[0, T.qAuto], [8000, '8 Mbps'], [4000, '4 Mbps'], [2000, '2 Mbps'], [1000, '1 Mbps']];
    return [h('h3', null, T.quality), ...q.map(([v, l]) => opt(l, maxBitrate === v, () => { maxBitrate = v; force = v ? 'transcode' : ''; start(pos()); }))];
  }
  function buildWhy() {
    const src = info.source || {};
    return [h('h3', null, T.whyTitle[mode] || mode),
      h('ul.why-list', null, (info.reasons || []).map(r => h('li', null, r))),
      info.encoder && h('p', { class: 'mt3' }, h('span.muted', null, T.encoder + ': '), info.encoder_label || info.encoder),
      info.bitrate && h('p.muted.small', null, `Cél: ${(info.bitrate / 1e6).toFixed(1).replace('.', ',')} Mbps, ${info.width} px széles`),
      h('p.muted.small', null, `Forrás: ${(src.container || '').toUpperCase()} · ${src.video_codec || '?'} ${src.width}×${src.height} · ${src.audio_codec || '–'}${src.hdr ? ' · ' + src.hdr : ''}`),
      h('p.muted.small', null, `Böngésző: ${caps.video.join(', ')} | ${caps.audio.join(', ')}`)];
  }
  function buildMore() {
    return [h('h3', null, item ? item.title : ''),
      opt(T.addToCollection, false, () => addToCollectionMenu(item, moreBtn)),
      opt(T.tags + '…', false, () => modal(T.tags, tagEditor(item), close => [h('button.btn', { onclick: close }, T.close)])),
      !partyCode && opt(T.startParty, false, async () => { const r = await POST('/api/party', { item_id: id }); exiting = true; await report(); go(`#/play/${id}?party=${r.code}&t=${Math.floor(pos())}`); }),
      state.me.user.is_admin && opt(T.markIntroStart + ` (${fmtTime(pos())})`, false, () => { markStart = pos(); toast(T.markIntroStart + ': ' + fmtTime(markStart)); }),
      state.me.user.is_admin && opt(T.markIntroEnd + ` (${fmtTime(pos())})`, false, () => {
        if (markStart == null || pos() <= markStart) return toast(T.markStartFirst, true);
        saveMark('intro', markStart, pos()); markStart = null;
      }),
      state.me.user.is_admin && opt(T.markCredits + ` (${fmtTime(pos())})`, false, () => saveMark('credits', pos(), duration || pos() + 60)),
      state.me.user.is_admin && segments.some(g => g.source === 'manual') && opt(T.clearIntro, false, async () => {
        await api('DELETE', `/api/items/${id}/segments/intro`); const f = await GET(`/api/items/${id}/segments`); segments = f.segments; toast(T.deleted);
      }),
    ];
  }
  ccBtn.onclick = () => info && openPop(buildTracks);
  gearBtn.onclick = () => info && openPop(buildQuality);
  whyBtn.onclick = () => info && openPop(buildWhy);
  moreBtn.onclick = () => item && openPop(buildMore);
  favBtn.onclick = async () => {
    if (!item) return; item.user_data.favorite = !item.user_data.favorite;
    await POST(`/api/items/${id}/favorite`, { favorite: item.user_data.favorite });
    favBtn.classList.toggle('on', item.user_data.favorite); put(favBtn, icon('heart', item.user_data.favorite));
  };

  // ---------------------------------------------------------------- timeline & controls
  function paintTimeline() {
    const d = duration || 1, p = pos();
    done.style.width = (p / d * 100) + '%';
    knob.style.left = (p / d * 100) + '%';
    const te = transcodedEnd();
    ready.style.left = (isHLS() ? offset / d * 100 : 0) + '%';
    ready.style.width = (Math.max(0, Math.min(d, te) - (isHLS() ? offset : 0)) / d * 100) + '%';
    timeEl.textContent = `${fmtTime(p)} / ${fmtTime(duration)}`;
    timeline.setAttribute('aria-valuemax', Math.round(duration));
    timeline.setAttribute('aria-valuenow', Math.round(p));
    timeline.setAttribute('aria-valuetext', fmtTime(p));
  }
  function chapterMarks() {
    timeline.querySelectorAll('.chap').forEach(c => c.remove());
    if (!duration) return;
    for (const c of info.chapters || []) if (c.start > 1) timeline.append(h('div.chap', { title: c.title, style: { left: (c.start / duration * 100) + '%' } }));
  }
  const tAt = e => { const r = timeline.getBoundingClientRect(); return Math.max(0, Math.min(1, ((e.clientX ?? e.touches?.[0]?.clientX) - r.left) / r.width)) * duration; };
  timeline.addEventListener('pointerdown', e => { dragging = true; timeline.setPointerCapture(e.pointerId); });
  timeline.addEventListener('pointermove', e => {
    const t = tAt(e);
    hoverT.classList.remove('hidden'); hoverT.style.left = (t / (duration || 1) * 100) + '%';
    const ch = (info?.chapters || []).filter(c => c.start <= t).pop();
    hoverT.textContent = fmtTime(t) + (ch && ch.title ? ' · ' + ch.title : '');
    if (dragging) { done.style.width = knob.style.left = (t / (duration || 1) * 100) + '%'; }
  });
  timeline.addEventListener('pointerleave', () => hoverT.classList.add('hidden'));
  timeline.addEventListener('pointerup', e => { if (dragging) { dragging = false; userSeek(tAt(e)); } });
  timeline.addEventListener('keydown', e => { if (e.key === 'ArrowLeft') { e.preventDefault(); e.stopPropagation(); seekBy(-10); } if (e.key === 'ArrowRight') { e.preventDefault(); e.stopPropagation(); seekBy(10); } });
  playBtn.onclick = togglePlay;
  video.addEventListener('click', togglePlay);
  video.addEventListener('dblclick', toggleFS);
  vol.oninput = () => { video.volume = +vol.value; video.muted = false; localStorage.setItem('kinora-vol', vol.value); paintVol(); };
  volBtn.onclick = () => { video.muted = !video.muted; paintVol(); };
  const paintVol = () => put(volBtn, icon(video.muted || video.volume === 0 ? 'mute' : 'vol'));
  fsBtn.onclick = toggleFS;
  function toggleFS() { if (document.fullscreenElement) document.exitFullscreen(); else player.requestFullscreen?.().catch(() => {}); }
  video.addEventListener('play', () => { put(playBtn, icon('pause')); playBtn.setAttribute('aria-label', T.pause); });
  video.addEventListener('pause', () => { put(playBtn, icon('play')); playBtn.setAttribute('aria-label', T.play); report(); });
  video.addEventListener('waiting', () => { put(mid, spinner); party?.buffering(true); });
  video.addEventListener('playing', () => { if (mid.firstChild === spinner) put(mid); party?.buffering(false); });
  video.addEventListener('canplay', () => { if (mid.firstChild === spinner && !video.paused) put(mid); party?.buffering(false); });
  video.addEventListener('loadeddata', () => { if (video.paused && mid.firstChild === spinner) put(mid); });
  video.addEventListener('timeupdate', () => {
    if (!dragging) paintTimeline();
    const now = Date.now();
    if (now - lastReport > 10000 && !video.paused) { lastReport = now; report(); }
    checkUpNext();
    checkSkip();
  });
  video.addEventListener('ended', () => {
    if (dead) return;
    if (isHLS() && transcodedEnd() < duration - 2) return; // HLS still producing
    report(true);
    if (nextItem && profile.autoplay_next && !party) playNext(); else if (!nextItem && !party) exit();
  });
  video.addEventListener('error', () => { if (!dead && mode === 'direct' && video.error && video.getAttribute('src')) { force = 'remux'; start(pos()); } });

  // idle hide
  let idleT;
  const wake = () => { player.classList.remove('idle'); clearTimeout(idleT); idleT = setTimeout(() => { if (!video.paused && pop.classList.contains('hidden')) player.classList.add('idle'); }, 3000); };
  stage.addEventListener('mousemove', wake); stage.addEventListener('touchstart', wake, { passive: true });
  wake();

  // keyboard
  const keys = e => {
    if (e.target.matches('input, textarea, select')) return;
    if (document.querySelector('.modal-back')) return;
    wake();
    switch (e.key) {
      case ' ': case 'k': e.preventDefault(); togglePlay(); break;
      case 'ArrowLeft': if (e.target !== timeline) { e.preventDefault(); seekBy(-10); } break;
      case 'ArrowRight': if (e.target !== timeline) { e.preventDefault(); seekBy(10); } break;
      case 'ArrowUp': e.preventDefault(); video.volume = Math.min(1, video.volume + 0.05); vol.value = video.volume; paintVol(); break;
      case 'ArrowDown': e.preventDefault(); video.volume = Math.max(0, video.volume - 0.05); vol.value = video.volume; paintVol(); break;
      case 'f': case 'F': toggleFS(); break;
      case 'm': case 'M': video.muted = !video.muted; paintVol(); break;
      case 'c': case 'C': if (info) openPop(buildTracks); break;
      case 'n': case 'N': if (nextItem) playNext(); break;
      case 'Escape': if (!pop.classList.contains('hidden')) pop.classList.add('hidden'); else if (!document.fullscreenElement) exit(); break;
    }
  };
  document.addEventListener('keydown', keys);

  // ---------------------------------------------------------------- progress & next episode
  async function report(ended) {
    if (!item || (!ended && pos() < 1)) return;
    try { await POST(`/api/items/${id}/progress`, { position: pos(), ended: !!ended }, { keepalive: true, quiet: true }); } catch { /* offline */ }
  }
  // ---------------------------------------------------------------- skip intro / recap / credits
  function currentSeg() {
    const p = pos();
    return segments.find(g => g.type !== 'preview' && p >= g.start && p < g.end - 1) || null;
  }
  function checkSkip() {
    const mode = profile.skip_intro || 'manual';
    let g = mode === 'off' ? null : currentSeg();
    if (g && g.type === 'credits' && nextItem) g = null; // the next-episode card takes over
    if (g && mode === 'auto' && !skipped.has(g.type + g.start) && !party) {
      skipped.add(g.type + g.start);
      toast(T.skippedIntro);
      userSeek(g.end);
      return;
    }
    if (g === shownSeg) return;
    shownSeg = g;
    skipBtn.classList.toggle('hidden', !g);
    if (g) skipBtn.textContent = g.type === 'credits' ? T.skipCredits : g.type === 'recap' ? T.skipRecap : T.skipIntro;
  }
  function doSkip() {
    const g = currentSeg();
    if (!g) return;
    skipped.add(g.type + g.start);
    skipBtn.classList.add('hidden'); shownSeg = null;
    if (party) party.send({ t: 'skip', pos: g.end, kind: g.type }); // everyone in the party jumps
    seekTo(g.end);
  }
  async function saveMark(type, start, end) {
    const r = await api('PUT', `/api/items/${id}/segments/${type}`, { start, end });
    toast(T.markSaved);
    const fresh = await GET(`/api/items/${id}/segments`).catch(() => null);
    if (fresh) segments = fresh.segments;
    if (r.can_propagate && await confirmBox(T.propagateAsk)) {
      const p = await POST(`/api/items/${id}/segments/${type}/propagate`, {});
      toast(T.propagated(p.updated));
    }
  }

  function checkUpNext() {
    if (!nextItem || nextShown || !duration) return;
    const at = credits ?? Math.max(0, duration - 30);
    if (pos() >= at) showUpNext();
  }
  function showUpNext() {
    nextShown = true;
    let n = 10;
    const count = h('span.count', null, T.nextIn(n));
    put(upnext, artImg(nextItem, 'thumb', nextItem.title), h('div.b', null,
      h('div.muted.small', null, T.nextEp + ' · ' + T.ep(nextItem.season_num, nextItem.episode_num)), h('b', null, nextItem.title),
      h('div', { class: 'mt2 mb3' }, profile.autoplay_next && !party ? count : null),
      h('div.row-gap', null, h('button.btn.primary', { onclick: playNext }, icon('play'), T.playNow), h('button.btn', { onclick: clearUpNext }, T.dismiss))));
    upnext.classList.remove('hidden');
    if (profile.autoplay_next && !party) nextTimer = setInterval(() => { n--; count.textContent = T.nextIn(n); if (n <= 0) playNext(); }, 1000);
  }
  function clearUpNext() { clearInterval(nextTimer); upnext.classList.add('hidden'); }
  async function playNext() {
    clearUpNext();
    if (!nextItem) return;
    await report(true);
    if (party) party.send({ t: 'item', item_id: nextItem.id, pos: 0 });
    exiting = true;
    go(`#/play/${nextItem.id}` + (partyCode ? `?party=${partyCode}` : ''));
  }

  async function exit() {
    if (exiting) return;
    exiting = true;
    await report();
    if (history.length > 1) history.back(); else go('#/item/' + id);
  }

  onCleanup(() => {
    dead = true;
    document.removeEventListener('keydown', keys);
    clearUpNext(); clearTimeout(idleT);
    if (!exiting) report();
    if (hls) hls.destroy();
    video.pause(); video.removeAttribute('src'); video.load();
    if (session) fetch('/api/hls/' + session, { method: 'DELETE', credentials: 'same-origin', keepalive: true }).catch(() => {});
    if (party) party.close();
    if (document.fullscreenElement) document.exitFullscreen().catch(() => {});
  });

  // ---------------------------------------------------------------- go
  let detail;
  try { detail = await GET('/api/items/' + id); }
  catch { return; }
  item = detail.item;
  nextItem = detail.next || null;
  nextBtn.classList.toggle('hidden', !nextItem);
  nextBtn.onclick = playNext;
  put(favBtn, icon('heart', item.user_data.favorite)); favBtn.classList.toggle('on', item.user_data.favorite);
  titleEl.textContent = item.kind === 'episode' ? item.series_title : item.title;
  subEl.textContent = item.kind === 'episode' ? `${T.ep(item.season_num, item.episode_num)} · ${item.title}` : [item.year, item.content_rating].filter(Boolean).join(' · ');
  document.title = item.title + ' – ' + T.appName;
  const resumeAt = q.has('t') ? +q.get('t') : (item.user_data.position || 0);
  if (partyCode) party = partyClient(partyCode, player, stage, { video, pos, seekTo, getItemId: () => id, onItem: nid => { exiting = true; go(`#/play/${nid}?party=${partyCode}`); } });
  await start(partyCode ? 0 : resumeAt, { paused: !!partyCode });
  if (info) {
    chooseInitialSubtitle(); applySubtitle(); chapterMarks();
    if (mode === 'transcode') toast(`${T.whyTitle[mode]}: ${(info.reasons || [])[0] || ''} – részletek: ⓘ`);
  }
}

// ===================================================================== watch party client
function partyClient(code, player, stage, p) {
  const panel = h('aside.party', { 'aria-label': T.party });
  const membersEl = h('div.members');
  const chat = h('div.chat', { 'aria-live': 'polite' });
  const status = h('div.status');
  const everyone = h('input', { type: 'checkbox' });
  const msg = h('input', { type: 'text', maxlength: 500, placeholder: T.partyChatPh, 'aria-label': T.partyChatPh });
  const link = location.origin + '/#/party/' + code;
  const reacts = h('div.reacts');
  panel.append(
    h('header', null, h('div.row-gap', { class: 'spread' }, h('div', null, h('div.code', { 'aria-label': T.party + ' ' + code }, code)),
      h('button.btn', { onclick: () => modal(T.partyLink, h('div', null, qrImg(link), h('p.mono', { class: 'mt3 break-all' }, link)), close => [h('button.btn', { onclick: () => copyText(link) }, T.copy), h('button.btn.primary', { onclick: close }, T.close)]) }, T.partyLink)),
      h('label.check.hidden', { class: 'mt3 mb0' }, everyone, T.partyEveryone), status),
    membersEl, chat, reacts,
    h('form', { onsubmit: e => { e.preventDefault(); if (msg.value.trim()) { send({ t: 'chat', text: msg.value }); msg.value = ''; } } }, msg, h('button.btn', { type: 'submit' }, T.partySend)));
  player.append(panel);

  let ws, myId = 0, offset = 0, bestRtt = Infinity, last = null, closed = false, retry = 0, pingT, syncT, applying = 0, bufState = false, lastSeq = 0;
  const serverNow = () => Date.now() + offset;
  const send = m => { if (ws && ws.readyState === 1) ws.send(JSON.stringify(m)); };
  everyone.onchange = () => send({ t: 'everyone', v: everyone.checked });

  function line(el) { chat.append(el); chat.scrollTop = chat.scrollHeight; }
  function connect() {
    const proto = location.protocol === 'https:' ? 'wss' : 'ws';
    ws = new WebSocket(`${proto}://${location.host}/api/party/${code}/ws`);
    ws.onopen = () => { retry = 0; status.textContent = ''; ping(); };
    ws.onmessage = ev => {
      const m = JSON.parse(ev.data);
      switch (m.t) {
        case 'hello': myId = m.id; put(reacts, ...m.reactions.map(e => h('button', { 'aria-label': e, onclick: () => send({ t: 'react', emoji: e }) }, e))); break;
        case 'pong': {
          const now = Date.now(), rtt = now - m.c;
          if (rtt <= bestRtt * 1.5 || rtt < 60) { bestRtt = Math.min(bestRtt, rtt); offset = m.s - (m.c + rtt / 2); }
          break;
        }
        case 'state': onState(m); break;
        case 'chat': line(h('div', null, h('b', { style: { color: m.color } }, m.name + ': '), m.text)); break;
        case 'sys': line(h('div.sys', null, m.text)); break;
        case 'react': {
          const f = h('div.float-react', { style: { left: (10 + Math.random() * 70) + '%' } }, m.emoji);
          stage.append(f); setTimeout(() => f.remove(), 2700); break;
        }
      }
    };
    ws.onclose = ev => {
      if (closed) return;
      if (ev.code === 1006 && retry > 4) { status.textContent = T.partyNotFound; return; }
      status.textContent = T.partyLost;
      setTimeout(connect, Math.min(8000, 500 * 2 ** retry++));
    };
  }
  function ping() { send({ t: 'ping', c: Date.now() }); }
  pingT = setInterval(ping, 5000);

  function onState(st) {
    last = st; lastSeq = st.seq;
    if (st.item_id !== p.getItemId()) { p.onItem(st.item_id); return; }
    const me = st.members.find(x => x.id === myId);
    everyone.closest('label').classList.toggle('hidden', !(me && me.host));
    everyone.checked = st.everyone;
    put(membersEl, h('div.muted.small', null, T.partyMembers), ...st.members.map(x => h('div.member', null, h('span.dot', { style: { background: x.color } }), x.name, x.host && h('span.muted.small', null, '· ' + T.host), x.buffering && h('span.small', { style: { color: 'var(--amber)' } }, '· ' + T.buffering))));
    status.textContent = st.waiting.length && st.intended ? T.partyWaiting(st.waiting) : '';
    sync(true);
  }
  // drift > 1.5 s → hard seek; 0.15–1.5 s → nudge playbackRate; otherwise 1.0
  function sync(fromState) {
    if (!last) return;
    const v = p.video;
    const target = last.pos + (last.playing ? (serverNow() - last.at) / 1000 : 0);
    if (last.playing && v.paused) { applying++; v.play().catch(() => {}).finally(() => setTimeout(() => applying--, 50)); }
    if (!last.playing && !v.paused) { applying++; v.pause(); setTimeout(() => applying--, 50); }
    const drift = target - p.pos();
    if (Math.abs(drift) > 1.5 && (fromState || last.playing)) { applying++; p.seekTo(target).finally(() => setTimeout(() => applying--, 50)); v.playbackRate = 1; }
    else if (last.playing && Math.abs(drift) > 0.15) v.playbackRate = drift > 0 ? 1.05 : 0.95;
    else v.playbackRate = 1;
  }
  syncT = setInterval(() => sync(false), 1000);
  connect();
  return {
    send: m => { if (applying) return; send(m); },
    buffering: v => { if (v !== bufState) { bufState = v; send({ t: 'buffering', v }); } },
    close: () => { closed = true; clearInterval(pingT); clearInterval(syncT); try { ws.close(); } catch { /* ignore */ } panel.remove(); },
  };
}

// ===================================================================== admin
async function viewAdmin(tab) {
  if (!state.me.user.is_admin) return go('#/home');
  const main = shell('admin');
  const tabs = [['dash', T.aDash], ['libraries', T.aLibs], ['users', T.aUsers], ['invites', T.aInvites], ['transcode', T.aTranscode], ['metadata', T.aMeta], ['connect', T.aConnect], ['devices', T.aDevices], ['settings', T.aSettings]];
  const body = h('div');
  main.append(h('div.page-head', null, h('h1', null, T.admin)), h('div.admin', null,
    h('nav.admin-nav', { 'aria-label': T.admin }, tabs.map(([k, l]) => h('a' + (k === tab ? '.on' : ''), { href: '#/admin/' + k }, l))), body));
  const fn = { dash: adminDash, libraries: adminLibs, users: adminUsers, invites: adminInvites, transcode: adminTranscode, metadata: adminMeta, connect: adminConnect, devices: adminDevices, settings: adminSettings }[tab];
  if (fn) await fn(body);
}

async function adminDash(body) {
  let t;
  const render = async () => {
    const d = await GET('/api/admin/dashboard');
    const up = d.uptime_sec, uptime = up > 86400 ? `${Math.floor(up / 86400)} nap ${Math.floor(up / 3600) % 24} ó` : `${Math.floor(up / 3600)} ó ${Math.floor(up / 60) % 60} p`;
    const stat = (v, l) => h('div.stat', null, h('b', null, v), h('span', null, l));
    put(body, 
      h('div.ledger', null, stat(d.library.movies, 'film'), stat(d.library.series, 'sorozat'), stat(d.library.episodes, 'epizód'), stat(fmtBytes(d.library.bytes), T.size),
        stat(d.transcodes.length, T.activeTranscodes), stat(d.parties.rooms, T.parties), stat(uptime, T.uptime), stat(d.version, T.version)),
      d.library.missing > 0 && h('p.muted', null, `${d.library.missing} fájl jelenleg hiányzik (nem törlődött – ha visszakerül, a megnézett állapot megmarad).`),
      h('div.panel', null, h('h2', null, T.activeTranscodes), d.transcodes.length ? h('table.tbl', null, h('tbody', null, d.transcodes.map(s => h('tr', null, h('td', null, s.title), h('td', null, s.mode), h('td', null, s.encoder), h('td', null, fmtTime(s.seconds)))))) : h('p.muted', null, 'Nincs.')),
      h('div.panel', null, h('h2', null, T.scans), scanTable(d.scans)),
      h('div.panel', null, h('h2', null, T.selectedEnc), h('p', null, d.encoder_selected.label), h('p.muted.small', null, T.tonemap + ': ' + (d.tonemap ? T.yes : T.no))),
      h('div.panel', null, h('h2', null, 'Mentés'), h('p.muted', null, T.backupHint), h('a.btn.primary', { href: '/api/admin/backup', download: '' }, icon('dl'), T.backup)));
    t = setTimeout(render, 5000);
  };
  onCleanup(() => clearTimeout(t));
  await render();
}
function scanTable(scans) {
  if (!scans || !scans.length) return h('p.muted', null, 'Még nem futott szkennelés.');
  const libName = id => (state.libs || []).find(l => l.id === id)?.name || '#' + id;
  return h('table.tbl', null, h('tbody', null, scans.map(s => h('tr', null, h('td', null, libName(s.library_id)),
    h('td', null, s.running ? h('div', null, `${T.phase[s.phase]} – ${s.done}/${s.total}`, h('div.bar', null, h('i', { style: { width: (s.total ? s.done / s.total * 100 : 5) + '%' } }))) : s.error ? h('span.bad', null, T.scanErr + ': ' + s.error) : `${s.changed} változás, ${s.removed} hiányzó, ${s.moved} áthelyezett · ${(s.took_ms / 1000).toFixed(1).replace('.', ',')} mp`),
    h('td', null, s.errors ? h('span.bad', { title: s.last_error }, `${s.errors} hibás fájl`) : '')))));
}

async function adminLibs(body) {
  let t;
  if (!state.installMode) state.installMode = (await GET('/api/admin/browse?path=/').catch(() => ({}))).mode || '';
  const render = async () => {
    const [libs, scans] = await Promise.all([GET('/api/libraries'), GET('/api/admin/scan')]);
    state.libs = libs;
    const scanOf = id => scans.find(s => s.library_id === id);
    put(body, h('div.panel', null, h('div.row-gap', { class: 'spread mb3' }, h('h2', { class: 'm0' }, T.aLibs), h('button.btn.primary', { onclick: addLib }, icon('plus'), T.addLibrary)),
      libs.length ? h('div.scroll-x', null, h('table.tbl', null, h('thead', null, h('tr', null, h('th', null, T.libName), h('th', null, T.libPath), h('th', null, T.libKind), h('th', null, T.lastScan), h('th', null, ''))),
        h('tbody', null, libs.map(l => {
          const s = scanOf(l.id);
          return h('tr', null, h('td', null, h('b', null, l.name), h('div.small.muted', null, `${l.count} cím`)), h('td.mono', null, l.path), h('td', null, T.kinds[l.kind]),
            h('td', { class: 'td-progress' }, s && s.running ? h('div', null, `${T.phase[s.phase]} – ${s.done}/${s.total}`, h('div.bar', null, h('i', { style: { width: (s.total ? s.done / s.total * 100 : 5) + '%' } })))
              : s && s.error ? h('span.bad', null, s.error) : l.last_scan_at ? T.ago(Date.now() / 1000 - l.last_scan_at) : T.never,
              s && s.errors ? h('div.small.bad', { title: s.last_error }, `${s.errors} hibás fájl: ${s.last_error}`) : null),
            h('td', null, h('div.row-gap', null, h('button.btn', { onclick: async () => { await POST(`/api/libraries/${l.id}/scan`); render(); } }, T.scan),
              h('button.btn', { onclick: async () => permDialog(await GET('/api/admin/permcheck?library=' + l.id)) }, T.permCheck),
              h('button.btn.danger', { onclick: async () => { if (await confirmBox(T.confirmDelete(l.name))) { await api('DELETE', '/api/libraries/' + l.id); state.libs = null; render(); } } }, T.delete))));
        })))) : h('p.muted', null, T.homeEmptyAdmin)));
    clearTimeout(t);
    if (scans.some(s => s.running)) t = setTimeout(render, 1500);
  };
  onCleanup(() => clearTimeout(t));
  await render();

  function addLib() {
    const name = h('input', { type: 'text' });
    const path = h('input', { type: 'text', placeholder: '/media/movies' });
    const kind = h('select', null, Object.entries(T.kinds).map(([k, v]) => h('option', { value: k }, v)));
    modal(T.addLibrary, h('div', null, field(T.libName, name), h('label.field', null, h('span', null, T.libPath), h('div.row-gap', { class: 'nowrap' }, path, h('button.btn', { type: 'button', onclick: () => folderPicker(path.value, p => { path.value = p; if (!name.value) name.value = p.split(/[\\/]/).filter(Boolean).pop() || ''; }) }, T.choose))),
      h('div.hint', { class: 'mt0' }, state.installMode === 'docker' ? T.mediaHintDocker : T.mediaHintNative), field(T.libKind, kind)), close => [h('button.btn', { onclick: close }, T.cancel),
      h('button.btn.primary', { onclick: async () => {
        try { await POST('/api/libraries', { name: name.value, path: path.value, kind: kind.value }, { quiet: true }); }
        catch (e) { if (e.body && e.body.perm) permDialog(e.body.perm); else toast(e.message, true); return; }
        close(); state.libs = null; render();
      } }, T.addLibrary)]);
  }
}

function permDialog(rep) {
  const copyBlock = lines => h('div', null, h('pre.code', null, lines.join('\n')), h('button.btn', { onclick: () => copyText(lines.join('\n')) }, T.copy));
  modal(T.permCheck, h('div', null,
    h('p', { class: rep.ok ? 'ok' : 'bad' }, rep.ok ? T.permOk : rep.message),
    !rep.ok && rep.problems.length > 0 && h('ul.small', null, rep.problems.map(pr => h('li.mono', null, `${pr.path}: ${pr.need === 'x' ? 'belépés (x)' : 'olvasás (rX)'}`))),
    !rep.ok && rep.commands.length > 0 && [h('p', { class: 'mt3' }, T.permFix), copyBlock(rep.commands)],
    !rep.ok && rep.group_commands && [h('p', { class: 'mt3' }, T.permGroup), copyBlock(rep.group_commands)],
    rep.hint && h('p.muted.small', { class: 'mt3' }, rep.hint)), close => [h('button.btn.primary', { onclick: close }, T.close)]);
}

async function folderPicker(start, onPick) {
  const list = h('div', { class: 'scroll-list' });
  const cur = h('div.mono');
  const shortcuts = h('div.chips', { 'aria-label': T.shortcuts });
  let current = '';
  const load = async p => {
    const r = await GET('/api/admin/browse' + (p ? '?path=' + encodeURIComponent(p) : ''));
    current = r.path; cur.textContent = r.path + (r.host_path && r.host_path !== r.path ? '  ·  ' + T.hostPath(r.host_path) : '');
    put(shortcuts, (r.shortcuts || []).map(sc => h('button.chip' + (sc === r.path ? '.on' : ''), { onclick: () => load(sc) }, sc)));
    const locked = new Set(r.locked || []);
    put(list, r.parent && h('button.opt', { onclick: () => load(r.parent) }, '↑ ' + T.up), ...r.dirs.map(d => h('button.opt', { onclick: () => load(r.path.replace(/[\\/]$/, '') + r.sep + d) },
      locked.has(d) && [icon('lock'), ' '], d, locked.has(d) && h('span.muted.small', null, ' · ' + T.locked))));
    if (!r.dirs.length) list.append(h('p.muted.small', null, 'Nincs almappa.'));
  };
  modal(T.pickFolder, h('div', null, shortcuts, cur, list), close => [h('button.btn', { onclick: close }, T.cancel),
    h('button.btn', { onclick: async () => permDialog(await GET('/api/admin/permcheck?path=' + encodeURIComponent(current))) }, T.permCheck),
    h('button.btn.primary', { onclick: () => { onPick(current); close(); } }, T.useFolder)]);
  await load(start || '').catch(() => load(''));
}

async function adminUsers(body) {
  const users = await GET('/api/admin/users');
  put(body, h('div.panel', null, h('div.row-gap', { class: 'spread mb3' }, h('h2', { class: 'm0' }, T.aUsers), h('button.btn.primary', { onclick: newUser }, icon('plus'), T.newUser)),
    h('div.scroll-x', null, h('table.tbl', null, h('tbody', null, users.map(u => h('tr', null,
      h('td', null, h('b', null, u.username), u.id === state.me.user.id && h('span.muted', null, ' ' + T.you)), h('td', null, T.profilesN(u.profiles)), h('td', null, u.is_admin ? T.isAdmin : ''),
      h('td', null, h('div.row-gap', null,
        h('button.btn', { onclick: () => resetPw(u) }, T.resetPw),
        u.id !== state.me.user.id && h('button.btn', { onclick: async () => { await api('PATCH', '/api/admin/users/' + u.id, { is_admin: !u.is_admin }); adminUsers(body); } }, u.is_admin ? 'Admin jog elvétele' : 'Admin jog'),
        u.id !== state.me.user.id && h('button.btn.danger', { onclick: async () => { if (await confirmBox(T.confirmDelete(u.username))) { await api('DELETE', '/api/admin/users/' + u.id); adminUsers(body); } } }, T.delete))))))))));
  function newUser() {
    const un = h('input', { type: 'text' }), pw = h('input', { type: 'password' }), adm = h('input', { type: 'checkbox' });
    modal(T.newUser, h('div', null, field(T.username, un), field(T.password, pw), h('label.check', null, adm, T.isAdmin)), close => [h('button.btn', { onclick: close }, T.cancel),
      h('button.btn.primary', { onclick: async () => { await POST('/api/admin/users', { username: un.value, password: pw.value, is_admin: adm.checked }); close(); adminUsers(body); } }, T.create)]);
  }
  function resetPw(u) {
    const pw = h('input', { type: 'password' });
    modal(T.resetPw + ': ' + u.username, field(T.password, pw), close => [h('button.btn', { onclick: close }, T.cancel),
      h('button.btn.primary', { onclick: async () => { await api('PATCH', '/api/admin/users/' + u.id, { password: pw.value }); close(); toast(T.saved); } }, T.save)]);
  }
}

async function adminInvites(body) {
  const invs = await GET('/api/admin/invites');
  const base = (await GET('/api/admin/settings')).values.public_url || location.origin;
  const linkOf = c => base + '/#/join/' + c;
  put(body, h('div.panel', null, h('div.row-gap', { class: 'spread mb3' }, h('h2', { class: 'm0' }, T.aInvites), h('button.btn.primary', { onclick: newInv }, icon('plus'), T.newInvite)),
    invs.length ? h('div', null, invs.map(i => h('div.entry', { style: { opacity: i.uses_left > 0 ? 1 : 0.55 } },
      qrImg(linkOf(i.code)),
      h('div', null, h('div.code', null, i.code), i.note && h('p', null, i.note),
        h('p.muted.small', null, T.usesLeft(i.uses_left), i.expires_at ? ` · ${T.expires}: ${new Date(i.expires_at * 1000).toLocaleString('hu-HU')}` : ''),
        h('p.mono.small', { class: 'break-all' }, linkOf(i.code)),
        h('div.row-gap', null, h('button.btn', { onclick: () => copyText(linkOf(i.code)) }, T.copy), h('button.btn.danger', { onclick: async () => { await api('DELETE', '/api/admin/invites/' + i.code); adminInvites(body); } }, T.delete)))))) : h('p.muted', null, 'Nincs meghívó.')));
  function newInv() {
    const note = h('input', { type: 'text', placeholder: 'pl. Nagyiéknak' }), uses = h('input', { type: 'number', value: 1, min: 1, max: 100 }), hours = h('input', { type: 'number', value: 72, min: 0 });
    modal(T.newInvite, h('div', null, field(T.note, note), h('div.grid2', null, field(T.uses, uses), field(T.validHours, hours))), close => [h('button.btn', { onclick: close }, T.cancel),
      h('button.btn.primary', { onclick: async () => { await POST('/api/admin/invites', { note: note.value, uses: +uses.value, hours: +hours.value }); close(); adminInvites(body); } }, T.create)]);
  }
}

function settingsForm(values, env, keys, onSaved) {
  const inputs = {};
  const rows = keys.map(([k, label, kind, opts]) => {
    const locked = env[k];
    let el;
    if (kind === 'select') el = h('select', { disabled: !!locked }, opts.map(([v, l]) => h('option', { value: v, selected: values[k] === v }, l)));
    else el = h('input', { type: kind || 'text', value: values[k] ?? '', disabled: !!locked });
    inputs[k] = el;
    return h('label.field', null, h('span', null, label), el, locked && h('div.env-lock', null, T.envLocked(locked)));
  });
  const save = h('button.btn.primary', { onclick: async () => {
    const body = {};
    for (const [k] of keys) if (!env[k]) body[k] = inputs[k].value;
    await api('PATCH', '/api/admin/settings', body); toast(T.saved); onSaved && onSaved();
  } }, T.save);
  return h('div', null, rows, save);
}

async function adminTranscode(body) {
  const [d, s] = await Promise.all([GET('/api/admin/dashboard'), GET('/api/admin/settings')]);
  const table = res => h('div.scroll-x', null, h('table.tbl', null, h('thead', null, h('tr', null, h('th', null, T.encoder), h('th', null, 'Eredmény'), h('th', null, 'Idő'), h('th', null, 'Részletek'))),
    h('tbody', null, res.map(r => h('tr', null, h('td', null, h('b', null, r.label), h('div.small.muted.mono', null, r.name)), h('td', { class: r.ok ? 'ok' : 'bad' }, r.ok ? 'működik' : 'nem elérhető'), h('td', null, r.ok ? r.ms + ' ms' : ''), h('td.small.muted', null, r.error || ''))))));
  const tbl = h('div', null, table(d.encoders || []));
  const btn = h('button.btn', { onclick: async () => { btn.disabled = true; btn.textContent = T.hwTesting; try { const r = await POST('/api/admin/hwtest'); put(tbl, table(r.results)); toast(T.selectedEnc + ': ' + r.selected.label); } finally { btn.disabled = false; btn.textContent = T.hwTest; } } }, T.hwTest);
  const encOpts = [['auto', T.encAuto], ...(d.encoders || []).filter(e => e.ok).map(e => [e.name, e.label])];
  put(body, 
    h('div.panel', null, h('h2', null, 'Encoder-önteszt'), h('p.muted', null, 'Minden encoder valóban lekódol 1 másodpercnyi tesztvideót – így az derül ki, ami tényleg működik, nem csak az, amit az ffmpeg ismer.'), btn, h('div', { class: 'mt3' }, tbl),
      h('p', { class: 'mt3' }, h('span.muted', null, T.selectedEnc + ': '), d.encoder_selected.label), h('p.muted.small', null, T.tonemap + ': ' + (d.tonemap ? T.yes : T.no))),
    h('div.panel', null, h('h2', null, T.encSetting), settingsForm(s.values, s.env, [['encoder', T.encSetting, 'select', encOpts]], () => adminTranscode(body))),
    await segmentsPanel(s));
}

async function segmentsPanel(s) {
  const st = await GET('/api/admin/segments').catch(() => null);
  const onoff = [['1', T.on], ['0', T.off]];
  return h('div.panel', null, h('h2', null, T.aSegments),
    st && h('dl.side-info', null,
      h('dt', null, T.segBackend), h('dd', { class: st.available ? 'ok' : 'bad' }, st.backend),
      st.stats && [h('dt', null, 'Állapot'), h('dd', null, T.segStats(st.stats.episodes, st.stats.analysed, st.stats.with_intro, st.stats.with_credits, st.stats.manual))],
      h('dt', null, T.segQueue), h('dd', null, String(st.queue || 0)),
      st.running && [h('dt', null, T.segRunning), h('dd', null, st.running)],
      st.last_error && [h('dt', null, 'Utolsó hiba'), h('dd.bad.small', null, st.last_error)]),
    settingsForm(s.values, s.env, [['intro_detection', T.segDetection, 'select', onoff], ['intro_window_min', T.segWindow, 'number']]));
}

async function adminMeta(body) {
  const [s, m] = await Promise.all([GET('/api/admin/settings'), GET('/api/admin/metadata')]);
  const retry = h('input', { type: 'checkbox' });
  const st = m.status;
  let t;
  put(body, 
    h('div.panel', null, h('h2', null, 'TMDB'), h('p.muted', null, T.metaHint),
      settingsForm(s.values, s.env, [['tmdb_key', T.tmdbKey, 'password'], ['metadata_lang', T.metaLang, 'select', [['hu-HU', 'Magyar'], ['en-US', 'Angol'], ['de-DE', 'Német']]]], () => adminMeta(body))),
    h('div.panel', null, h('h2', null, T.refreshMeta), h('label.check', null, retry, T.retryNoMatch),
      h('button.btn.primary', { disabled: !m.enabled, onclick: async () => { await POST('/api/admin/metadata/refresh', { retry_no_match: retry.checked }); adminMeta(body); } }, T.refreshMeta),
      h('p.muted', { class: 'mt3' }, T.metaStatus + ': ', st.running ? `fut – ${st.done}/${st.total}` : st.finished_at ? `kész (${st.matched} találat, ${st.no_match} nincs találat)` : '–', st.last_error ? h('div.bad.small', null, st.last_error) : null)));
  if (st.running) { t = setTimeout(() => adminMeta(body), 2000); onCleanup(() => clearTimeout(t)); }
}

async function adminConnect(body) {
  const c = await GET('/api/admin/connect');
  const card = (title, url, sub) => h('div.entry', null, qrImg(url),
    h('div', null, h('b', null, title), h('p.mono', { class: 'break-all' }, url), sub && h('p.muted.small', null, sub), h('button.btn', { onclick: () => copyText(url) }, T.copy)));
  const kindLabel = { lan: 'Helyi hálózat', tailscale: 'Tailscale', other: 'Egyéb' };
  put(body, 
    h('div.panel', null, h('h2', null, T.aConnect), h('p.muted', null, T.connectLede),
      h('div', null, c.public_url ? card('Nyilvános URL', c.public_url) : h('p.muted', null, T.noPublic),
        c.addresses.map(a => card(kindLabel[a.kind] + ' (' + a.iface + ')', a.url))),
      c.in_container && h('p.muted.small', { class: 'mt3' }, 'Dockerben futsz: a fenti címek a konténer belső címei lehetnek. Kívülről a gazdagép IP-címén és a kiajánlott porton (alapból 8096) éred el.')),
    h('div.panel', null, h('h2', null, 'Tailscale (ajánlott, portnyitás nélkül)'), h('p', null, 'Telepítsd a Tailscale-t a szerverre és a telefonodra ugyanazzal a fiókkal, vagy használd a docker-compose.tailscale.yml sidecart. Ezután a 100.x.y.z címen bárhonnan eléred.'),
      h('pre.code', null, 'TS_AUTHKEY=tskey-... docker compose -f docker-compose.yml -f docker-compose.tailscale.yml up -d')),
    h('div.panel', null, h('h2', null, 'Cloudflare Tunnel (saját domain, HTTPS)'), h('p', null, 'Hozz létre egy tunnelt a Cloudflare Zero Trust felületén, a public hostname mutasson a http://kinora:8096 címre, majd:'),
      h('pre.code', null, 'CF_TUNNEL_TOKEN=... docker compose -f docker-compose.yml -f docker-compose.cloudflared.yml up -d'), h('p.muted.small', null, 'Állítsd be a Nyilvános URL-t a Beállításokban, hogy a meghívó linkek is ezt használják.')),
    h('div.panel', null, h('h2', null, 'Reverse proxy (Caddy / nginx)'), h('p', null, 'A watch party WebSocketet használ, ezért az Upgrade fejléceket tovább kell adni.'),
      h('pre.code', null, 'kinora.pelda.hu {\n  reverse_proxy 127.0.0.1:8096\n}'),
      h('pre.code', null, 'location / {\n  proxy_pass http://127.0.0.1:8096;\n  proxy_http_version 1.1;\n  proxy_set_header Upgrade $http_upgrade;\n  proxy_set_header Connection "upgrade";\n  proxy_set_header Host $host;\n  proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;\n  proxy_set_header X-Forwarded-Proto $scheme;\n  proxy_buffering off;\n  client_max_body_size 0;\n}')));
}

async function adminDevices(body) {
  const r = await GET('/api/admin/devices');
  put(body, h('div.panel', null, h('h2', null, T.aDevices), r.devices.map(d => deviceRow(d, () => adminDevices(body), true))));
}

async function adminSettings(body) {
  const s = await GET('/api/admin/settings');
  const onoff = [['1', T.on], ['0', T.off]];
  put(body, 
    h('div.panel', null, h('h2', null, T.aSettings), settingsForm(s.values, s.env, [['server_name', T.serverName], ['public_url', T.publicUrl, 'url'], ['scan_interval_min', T.scanInterval, 'number'],
      ['quickconnect_enabled', T.qcEnabled, 'select', onoff], ['discovery_enabled', T.discEnabled, 'select', onoff]], () => { state.status = null; })),
    h('div.panel', null, h('h2', null, 'Mentés'), h('p.muted', null, T.backupHint), h('a.btn.primary', { href: '/api/admin/backup', download: '' }, icon('dl'), T.backup)));
}
