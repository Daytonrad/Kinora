// Kinora browser end-to-end test (Playwright).
//   BASE=http://127.0.0.1:18097 OUT=screenshots node scripts/e2e.mjs
// Needs a fresh server with the test library mounted at /media (see scripts/smoke.sh),
// and `npm i playwright`. Uses the system Edge/Chrome (Playwright's Chromium has no H.264/AAC).
import { chromium } from 'playwright';
import fs from 'node:fs';
import path from 'node:path';

const BASE = process.env.BASE || 'http://127.0.0.1:18097';
const OUT = process.env.OUT || 'screenshots';
const CHANNEL = process.env.CHANNEL || 'msedge';
const MEDIA = process.env.MEDIA || '/media';
fs.mkdirSync(OUT, { recursive: true });

let failures = 0;
const ok = m => console.log('  ✓ ' + m);
const bad = m => { console.log('  ✗ ' + m); failures++; };
const step = m => console.log('==> ' + m);
const shot = (page, name) => page.screenshot({ path: path.join(OUT, name + '.png') });

const browser = await chromium.launch({ channel: CHANNEL, args: ['--autoplay-policy=no-user-gesture-required'] });
const ctxA = await browser.newContext({ viewport: { width: 1440, height: 900 } });
const page = await ctxA.newPage();
const consoleErrors = [];
page.on('pageerror', e => consoleErrors.push(String(e)));
page.on('console', m => { if (m.type() === 'error' && !/Failed to load resource/.test(m.text())) consoleErrors.push(m.text()); });

const videoState = p => p.evaluate(() => { const v = document.querySelector('video'); return v ? { t: v.currentTime, paused: v.paused, rs: v.readyState, src: v.currentSrc } : null; });
async function waitPlaying(p, label, min = 1.5) {
  for (let i = 0; i < 60; i++) {
    const s = await videoState(p);
    if (s && s.t > min && !s.paused) return s;
    await p.waitForTimeout(500);
  }
  bad(label + ': video did not start'); return null;
}
async function why(p) {
  await p.mouse.move(300 + Math.random() * 200, 300); // wake the auto-hidden controls
  await p.click('button[aria-label="Miért így megy?"]');
  const text = await p.locator('.p-pop').innerText();
  return text;
}

try {
  step('setup wizard');
  await page.goto(BASE + '/');
  await page.waitForSelector('text=Állítsuk be a Kinorát');
  await shot(page, '01-setup');
  const inputs = page.locator('form input');
  await inputs.nth(0).fill('Otthoni mozi');
  await inputs.nth(1).fill('admin');
  await inputs.nth(2).fill('secret123');
  await inputs.nth(3).fill('secret123');
  await page.click('button:has-text("Admin fiók létrehozása")');

  step('first-run wizard');
  await page.waitForSelector('text=Hol vannak a filmjeid és sorozataid?');
  ok('admin created, wizard opens on the media step');
  await shot(page, '02-wizard-media');
  await page.click('.wiz button:has-text("Tallózás")');
  await page.waitForSelector('.modal');
  await page.click('.modal button.opt:has-text("Movies")');
  await page.waitForTimeout(400);
  await shot(page, '03-folder-picker');
  await page.click('.modal button:has-text("Ezt a mappát választom")');
  await page.waitForSelector('.modal', { state: 'detached' });
  const pathVal = await page.locator('.wiz-form input').nth(0).inputValue();
  pathVal.endsWith('Movies') ? ok('picker filled ' + pathVal) : bad('picker path ' + pathVal);
  await page.locator('.wiz-form input').nth(1).fill('Filmek');
  await page.locator('.wiz-form select').selectOption('movies');
  await page.click('.wiz-form button:has-text("Könyvtár hozzáadása")');
  await page.waitForSelector('.wiz-list li:has-text("Filmek")');
  ok('library added in the wizard');
  await page.click('.wiz-foot button:has-text("Tovább")');
  await page.waitForSelector('text=Poszterek, leírások, szereplők');
  await shot(page, '04-wizard-metadata');
  await page.click('.wiz-foot button:has-text("Kihagyom")');
  await page.waitForSelector('.wiz h1:has-text("Hardveres gyorsítás")');
  await page.waitForSelector('.wiz-body .wiz-list li', { timeout: 60000 });
  ok('encoder test ran: ' + (await page.locator('.wiz-body > div:last-child > p').first().innerText()));
  await shot(page, '05-wizard-hardware');
  await page.click('.wiz-foot button:has-text("Tovább")');
  await page.waitForSelector('text=Kérések és letöltések');
  await page.click('.wiz-foot button:has-text("Kihagyom")');
  await page.waitForSelector('text=Elérés otthonon kívülről');
  await page.locator('.wiz-form input').fill('ftp://rossz');
  await page.click('.wiz-foot button:has-text("Tovább")');
  await page.waitForTimeout(500);
  (await page.locator('text=Elérés otthonon kívülről').count()) ? ok('a bad public URL keeps the remote step open') : bad('bad public URL accepted');
  await page.locator('.wiz-form input').fill('');
  await page.click('.wiz-foot button:has-text("Tovább")');
  await page.waitForSelector('text=Kész is vagyunk');
  const sum = await page.locator('.wiz-sum').innerText();
  /Könyvtárak: 1/.test(sum) && /Nincs TMDB-kulcs/.test(sum) && /nincs beállítva/.test(sum) ? ok('summary: ' + sum.replace(/\s+/g, ' ')) : bad('summary: ' + sum);
  await shot(page, '06-wizard-summary');
  await page.click('.wiz-foot button:has-text("Kezdjük!")');
  await page.waitForSelector('text=Ki néz?');
  ok('wizard finished, profile picker shown');
  await shot(page, '07-profiles');
  await page.click('.who-item >> nth=0');
  await page.waitForSelector('.hero h1, .rail, .empty-note');

  step('add a library on the admin page');
  await page.goto(BASE + '/#/admin/libraries');
  await page.waitForSelector('.admin-nav');
  await page.click('button:has-text("Könyvtár hozzáadása")');
  await page.waitForSelector('.modal');
  await page.locator('.modal input').nth(0).fill('Sorozatok');
  await page.locator('.modal input').nth(1).fill(MEDIA + '/Shows');
  await page.locator('.modal select').selectOption('shows');
  await page.click('.modal button:has-text("Könyvtár hozzáadása")');
  await page.waitForTimeout(1200);
  await shot(page, '08-admin-libraries-scanning');
  for (let i = 0; i < 60; i++) {
    const r = await page.evaluate(() => fetch('/api/admin/scan').then(r => r.json()));
    if (r.length === 2 && r.every(s => !s.running)) break;
    await page.waitForTimeout(1000);
  }
  ok('scans finished');
  const dash = await page.evaluate(() => fetch('/api/admin/setup/status').then(r => r.json()));
  dash.pending === false ? ok('wizard no longer pending') : bad('wizard still pending: ' + JSON.stringify(dash));

  step('home');
  await page.goto(BASE + '/#/home');
  await page.waitForSelector('.hero h1');
  await page.waitForTimeout(800);
  await shot(page, '05-home');
  const rows = await page.locator('.shelf h2').allInnerTexts();
  rows.some(r => r.startsWith('Új:')) ? ok('rows: ' + rows.join(' | ')) : bad('home rows ' + rows);

  step('library browser + filters');
  await page.click('nav >> text=Movies');
  await page.waitForSelector('.grid .card');
  const before = await page.locator('.grid .card').count();
  await page.click('.chip:has-text("Sci-Fi")');
  await page.waitForTimeout(700);
  const after = await page.locator('.grid .card').count();
  before === 3 && after === 2 ? ok(`genre filter 3 → ${after}`) : bad(`filter ${before} → ${after}`);
  await shot(page, '06-library');

  step('detail page');
  await page.click('.grid .card:has-text("Big Test Movie")');
  await page.waitForSelector('.detail h1:has-text("Big Test Movie")');
  await page.locator('.tags input').fill('Kedvenc teszt');
  await page.locator('.tags input').press('Enter');
  await page.waitForSelector('.tags .chip:has-text("Kedvenc teszt")');
  ok('tag added');
  await shot(page, '07-detail');

  step('play – direct');
  await page.click('.detail .btn.primary');
  let s = await waitPlaying(page, 'direct');
  if (s) {
    const w = await why(page);
    w.includes('Közvetlen') ? ok('direct play, "why" panel: ' + w.split('\n')[1]) : bad('why: ' + w);
    await page.mouse.move(700, 450);
    await shot(page, '08-player-direct');
    await page.keyboard.press('Escape');
    // subtitle menu shows the Hungarian CP1250 subtitle
    await page.keyboard.press('c');
    const tracks = await page.locator('.p-pop').innerText();
    tracks.includes('Magyar') ? ok('subtitle menu lists Magyar') : bad('tracks: ' + tracks);
    await page.click('.p-pop .opt:has-text("Magyar")');
    await page.waitForTimeout(1500);
    const cue = await page.evaluate(() => { const t = document.querySelector('video').textTracks[0]; return t && t.cues ? [...t.cues].map(c => c.text).join(' | ') : ''; });
    cue.includes('Árvíztűrő') && cue.split('|').length === 2 ? ok('subtitle cues: ' + cue.replace(/\s+/g, ' ')) : bad('cues (expected exactly 2): ' + cue);
    await page.evaluate(() => { document.querySelector('video').currentTime = 2; });
    await page.waitForTimeout(800);
    await shot(page, '09-player-subtitle');
  }
  await page.keyboard.press('Escape');
  await page.keyboard.press('Escape');
  await page.waitForTimeout(800);

  step('play – transcode (HEVC/AC3)');
  const space = await page.evaluate(() => fetch('/api/search?q=Space').then(r => r.json()).then(j => j[0].id));
  await page.goto(`${BASE}/#/play/${space}`);
  s = await waitPlaying(page, 'transcode', 2);
  if (s) {
    const w = await why(page);
    w.includes('Átkódolás') && w.includes('hevc') ? ok('transcode reasons shown') : bad('why: ' + w);
    await page.mouse.move(700, 450);
    await shot(page, '10-player-transcode-why');
    await page.keyboard.press('Escape');
    // seek beyond the transcoded range → new session
    const sess1 = await page.evaluate(() => performance.getEntriesByType('resource').filter(e => e.name.includes('/api/hls/')).map(e => e.name.split('/')[5]).pop());
    await page.evaluate(() => {
      const tl = document.querySelector('.timeline'); const r = tl.getBoundingClientRect();
      const opts = { bubbles: true, clientX: r.left + r.width * 0.85, clientY: r.top + 5, pointerId: 1 };
      tl.dispatchEvent(new PointerEvent('pointerdown', opts)); tl.dispatchEvent(new PointerEvent('pointerup', opts));
    });
    await page.waitForTimeout(4000);
    const t = await page.locator('.p-time').innerText();
    const sec = t.split('/')[0].trim().split(':').reduce((a, b) => a * 60 + +b, 0);
    sec >= 30 ? ok(`seek outside transcoded range → new session, now at ${t}`) : bad('seek result ' + t + ' session ' + sess1);
  }

  step('episode → next episode card');
  const ep1 = await page.evaluate(() => fetch('/api/search?q=Pilot').then(r => r.json()).then(j => j.find(i => i.title === 'Pilot Part 1').id));
  await page.goto(`${BASE}/#/play/${ep1}`);
  s = await waitPlaying(page, 'remux');
  if (s) {
    await page.evaluate(() => { document.querySelector('video').currentTime = 12; });
    await page.waitForSelector('.upnext:not(.hidden)', { timeout: 15000 }).then(() => ok('"Következő rész" card at credits/last 30 s')).catch(() => bad('no up-next card'));
    await page.mouse.move(700, 450);
    await shot(page, '11-player-upnext');
    await page.click('.upnext button:has-text("Indítás most")');
    await page.waitForFunction(id => !location.hash.includes('/play/' + id), ep1);
    await page.waitForSelector('.p-head .s:has-text("S01E02")');
    ok('advanced to S01E02');
  }

  step('series detail + next up');
  await page.goto(BASE + '/#/home');
  await page.waitForSelector('.shelf');
  const rows2 = await page.locator('.shelf h2').allInnerTexts();
  rows2.includes('Folytatás') ? ok('rows now: ' + rows2.join(' | ')) : bad('rows ' + rows2);
  await shot(page, '12-home-continue');
  const showId = await page.evaluate(() => fetch('/api/search?q=Test Show').then(r => r.json()).then(j => j.find(i => i.kind === 'series').id));
  await page.goto(`${BASE}/#/item/${showId}`);
  await page.waitForSelector('.ep');
  await shot(page, '13-series');

  step('smart collection editor');
  await page.goto(BASE + '/#/collections/new?smart=1');
  await page.waitForSelector('text=Szabályok');
  await page.locator('.panel input').first().fill('Sci-Fi esték');
  await page.click('button:has-text("Szabály hozzáadása")');
  await page.waitForTimeout(800);
  const prev = await page.locator('h2:has-text("Előnézet")').innerText();
  prev.includes('2') ? ok(prev) : bad(prev);
  await shot(page, '14-smart-editor');
  await page.click('button:has-text("Mentés")');
  await page.waitForSelector('.page-head h1:has-text("Sci-Fi esték")');
  ok('smart collection saved');

  step('admin pages');
  for (const [tab, name] of [['dash', '15-admin-dash'], ['transcode', '16-admin-transcode'], ['connect', '17-admin-connect'], ['invites', '18-admin-invites']]) {
    await page.goto(`${BASE}/#/admin/${tab}`);
    await page.waitForSelector('.a-body:not([aria-busy]) .a-head');
    if (tab === 'invites') { await page.click('text=Új meghívó'); await page.click('.modal button:has-text("Létrehozás")'); await page.waitForSelector('.qr img'); }
    if (tab === 'transcode') { await page.click('text=Encoder-önteszt futtatása'); await page.waitForSelector('.a-tbl .ok'); }
    await page.waitForTimeout(400);
    await shot(page, name);
  }
  ok('admin pages render');

  step('branding: logo upload');
  await page.goto(`${BASE}/#/admin/settings`);
  await page.waitForSelector('.logo-slots');
  const logoPng = fs.readFileSync(new URL('../web/static/icons/icon-512.png', import.meta.url)); // a real PNG from the repo
  const fileInput = page.locator('.logo-slot input[type=file]').first();
  await fileInput.setInputFiles({ name: 'logo.png', mimeType: 'image/png', buffer: logoPng });
  await page.waitForSelector('.logo-preview.dark img', { timeout: 15000 }).then(() => ok('logo uploaded and previewed'), () => bad('logo preview missing'));
  await page.goto(`${BASE}/#/home`);
  await page.reload();
  await page.waitForSelector('header .logo.has-img img', { timeout: 10000 }).then(() => ok('top bar shows the server logo'), () => bad('logo not in top bar'));
  const fav = await page.evaluate(() => document.querySelector('link[rel="icon"]').getAttribute('href'));
  fav === '/api/branding/favicon.png' ? ok('favicon from branding') : bad('favicon: ' + fav);
  await page.goto(`${BASE}/#/admin/settings`);
  await page.waitForSelector('.logo-slots');
  await page.waitForTimeout(400);
  await shot(page, '15b-admin-branding');

  step('watch party with two tabs');
  const big = await page.evaluate(() => fetch('/api/search?q=Big').then(r => r.json()).then(j => j[0].id));
  const party = await page.evaluate(id => fetch('/api/party', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ item_id: id }) }).then(r => r.json()), big);
  await page.goto(`${BASE}/#/play/${big}?party=${party.code}`);
  await page.waitForSelector('.party .code');
  const ctxB = await browser.newContext({ viewport: { width: 1280, height: 800 }, storageState: await ctxA.storageState() });
  const pageB = await ctxB.newPage();
  await pageB.goto(`${BASE}/#/party/${party.code}`);
  await pageB.waitForSelector('.party .member >> nth=1');
  ok('second tab joined party ' + party.code);
  await page.waitForTimeout(1500);
  await page.click('.p-bar button[aria-label="Lejátszás"]').catch(() => page.keyboard.press(' '));
  await waitPlaying(pageB, 'party follower', 3);
  await page.waitForTimeout(3000);
  const [a, b] = [await videoState(page), await videoState(pageB)];
  Math.abs(a.t - b.t) < 1.0 ? ok(`in sync: A=${a.t.toFixed(2)} B=${b.t.toFixed(2)}`) : bad(`drift A=${a.t} B=${b.t}`);
  await pageB.locator('.party form input').fill('Szia! 🍿');
  await pageB.locator('.party form input').press('Enter');
  await page.waitForSelector('.party .chat >> text=Szia!');
  ok('chat delivered');
  await page.click('.reacts button >> nth=6');
  await page.keyboard.press(' ');
  await pageB.waitForFunction(() => document.querySelector('video').paused, null, { timeout: 5000 }).then(() => ok('pause propagated')).catch(() => bad('pause not propagated'));
  await page.mouse.move(600, 400);
  await shot(page, '19-party-host');
  await shot(pageB, '20-party-guest');
  // the guest leaves with the dedicated button and lands on home; back does not bounce into the party
  await pageB.click('.party-leave');
  await pageB.waitForFunction(() => location.hash === '#/home', null, { timeout: 8000 }).then(() => ok('"Kilépés a watch partyból" → home')).catch(() => bad('party leave did not go home'));
  await pageB.goBack().catch(() => {});
  await pageB.waitForTimeout(1500);
  const stuck = await pageB.evaluate(() => location.hash.indexOf('party=') >= 0);
  stuck ? bad('back bounced into the party again') : ok('back after leaving does not re-enter the party');
  await ctxB.close();

  step('search groups episodes of one series');
  await page.goto(`${BASE}/#/search?q=Pilot`);
  await page.waitForSelector('.card', { timeout: 10000 });
  const cards = await page.$$eval('.card', els => els.length);
  cards === 1 ? ok('three "Pilot" episodes → one series card') : bad('search cards: ' + cards);

  step('phone layout');
  const phone = await browser.newContext({ viewport: { width: 390, height: 844 }, deviceScaleFactor: 2, storageState: await ctxA.storageState(), isMobile: true, hasTouch: true });
  const pp = await phone.newPage();
  await pp.goto(BASE + '/#/home');
  await pp.waitForSelector('.hero h1');
  await pp.waitForTimeout(600);
  const overflow = await pp.evaluate(() => document.documentElement.scrollWidth > innerWidth + 1);
  overflow ? bad('horizontal page scroll on phone') : ok('no horizontal page scroll on phone');
  await shot(pp, '21-phone-home');
  await pp.goto(`${BASE}/#/item/${big}`);
  await pp.waitForSelector('.detail h1');
  await shot(pp, '22-phone-detail');
  await phone.close();
} catch (e) {
  bad('exception: ' + (e.stack || e));
  await shot(page, 'zz-failure').catch(() => {});
}
if (consoleErrors.length) { console.log('console errors:'); consoleErrors.slice(0, 15).forEach(e => console.log('   ' + e)); }
await browser.close();
console.log(failures ? `E2E FAILED (${failures})` : 'E2E OK');
process.exit(failures ? 1 : 0);
