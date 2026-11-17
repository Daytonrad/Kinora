// Browser E2E for the app UI against a running Kinora server.
//   SERVER=http://localhost:18098 ADMIN_PASS=secret123 node test/e2e.mjs
// TV run: 1920×1080 from file:// (exactly how Tizen/webOS load the app: Origin null,
// classic script), remote simulated with arrow keys / Enter / Escape, login via Quick
// Connect approved through the server API. Phone run: 390×844 touch over http://localhost.
// Screenshots land in apps/shared/test/out/.
import { chromium } from 'playwright-core';
import { createServer } from 'node:http';
import { readFileSync, existsSync, mkdirSync } from 'node:fs';
import { join, dirname, extname } from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';

const here = dirname(fileURLToPath(import.meta.url));
const dist = join(here, '..', 'dist');
const out = join(here, 'out');
mkdirSync(out, { recursive: true });
const SERVER = process.env.SERVER || 'http://localhost:18098';
const USER = process.env.ADMIN_USER || 'admin';
const PASS = process.env.ADMIN_PASS || 'secret123';
const host = SERVER.replace(/^https?:\/\//, '');

const browserPath = [process.env.BROWSER,
  'C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe',
  'C:/Program Files/Google/Chrome/Application/chrome.exe',
  '/usr/bin/google-chrome', '/usr/bin/chromium', '/usr/bin/microsoft-edge'].filter(p => p && existsSync(p))[0];
if (!browserPath) throw new Error('no Chromium-based browser found (set BROWSER=…)');

let failures = 0;
const ok = (m) => console.log('  \x1b[32m✓\x1b[0m ' + m);
const bad = (m) => { failures++; console.log('  \x1b[31m✗\x1b[0m ' + m); };
const check = (cond, m) => (cond ? ok(m) : bad(m));

async function adminCookie() {
  const r = await fetch(SERVER + '/api/login', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ username: USER, password: PASS }) });
  const tok = (await r.json()).token;
  const auth = { Authorization: 'Bearer ' + tok, 'Content-Type': 'application/json' };
  const me = await (await fetch(SERVER + '/api/me', { headers: auth })).json();
  await fetch(SERVER + `/api/profiles/${me.profiles[0].id}/select`, { method: 'POST', headers: auth, body: '{}' });
  return { auth, profile: me.profiles[0].id };
}

async function press(page, key, times = 1) {
  for (let i = 0; i < times; i++) { await page.keyboard.press(key); await page.waitForTimeout(120); }
}
const focused = (page) => page.evaluate(() => {
  const a = document.activeElement;
  return a ? (a.getAttribute('aria-label') || a.textContent || a.tagName).trim().slice(0, 60) : '';
});

const browser = await chromium.launch({ executablePath: browserPath, args: ['--autoplay-policy=no-user-gesture-required', '--allow-file-access-from-files'] });

// ---------------------------------------------------------------- TV
console.log('TV (1920×1080, file://, D-pad)');
{
  const ctx = await browser.newContext({ viewport: { width: 1920, height: 1080 }, deviceScaleFactor: 1 });
  const page = await ctx.newPage();
  const errors = [];
  page.on('pageerror', e => errors.push(e.message));
  await page.goto(pathToFileURL(join(dist, 'index.html')).href + '?mode=tv');
  await page.waitForSelector('input.addr');
  check(await page.evaluate(() => document.documentElement.className) === 'mode-tv', 'TV mode');
  await page.screenshot({ path: join(out, 'tv-01-server.png') });
  await page.fill('input.addr', host);
  await press(page, 'Enter');
  await page.waitForSelector('.code-big', { timeout: 15000 });
  const code = (await page.textContent('.code-big')).replace(/\D/g, '');
  check(/^\d{6}$/.test(code), 'Quick Connect code shown: ' + code);
  check(await page.$('.qr svg') !== null, 'QR code rendered');
  await page.screenshot({ path: join(out, 'tv-02-quickconnect.png') });
  const { auth } = await adminCookie();
  const appr = await fetch(SERVER + '/api/quickconnect/approve', { method: 'POST', headers: auth, body: JSON.stringify({ code }) });
  check(appr.ok, 'approved from another device');
  await page.waitForSelector('.who', { timeout: 15000 });
  await page.screenshot({ path: join(out, 'tv-03-profiles.png') });
  await press(page, 'Enter');
  await page.waitForSelector('.facade', { timeout: 15000 });
  await page.waitForTimeout(600);
  check((await page.evaluate(() => document.activeElement && document.activeElement.className)).indexOf('card') >= 0, 'first card focused on home');
  const t0 = await page.textContent('.facade .display');
  await press(page, 'ArrowRight', 2);
  await page.waitForTimeout(400);
  const t1 = await page.textContent('.facade .display');
  check(t0 !== t1 || (await page.$$('.shelf:first-of-type .card')).length < 3, `facade follows focus (${t0} → ${t1})`);
  await page.screenshot({ path: join(out, 'tv-04-home.png') });
  await press(page, 'ArrowDown');
  await page.waitForTimeout(300);
  check((await focused(page)).length > 0, 'down moves to next row: ' + await focused(page));
  await press(page, 'ArrowLeft', 6);
  check((await focused(page)) !== '', 'left reaches rail: ' + await focused(page));
  await page.screenshot({ path: join(out, 'tv-05-rail.png') });
  // open a movie detail through search results → use direct route for determinism
  const items = await (await fetch(SERVER + '/api/search?q=Big', { headers: auth })).json();
  await page.evaluate(id => { location.hash = '#/item/' + id; }, items[0].id);
  await page.waitForSelector('.detail-head');
  await page.waitForTimeout(400);
  check(/Lejátszás|Folytatás/.test(await focused(page)), 'detail: play focused: ' + await focused(page));
  await page.screenshot({ path: join(out, 'tv-06-detail.png') });
  await press(page, 'Enter');
  await page.waitForSelector('.player', { timeout: 10000 });
  await page.waitForFunction(() => { const v = document.querySelector('video'); return v && v.currentTime > 0.5; }, null, { timeout: 20000 }).then(() => ok('direct play runs'), () => bad('video did not start'));
  await page.waitForTimeout(500);
  // the test movie has an "Intro" chapter: the skip button shows and takes focus; OK skips it
  const skip = await page.evaluate(() => { const b = document.querySelector('.skip'); return b && b.style.display !== 'none' ? { text: b.textContent, focused: document.activeElement === b } : null; });
  check(skip && skip.text === 'Intró átugrása' && skip.focused, 'skip intro button shown and focused: ' + JSON.stringify(skip));
  await page.screenshot({ path: join(out, 'tv-07a-skip.png') });
  const s0 = await page.evaluate(() => document.querySelector("video").currentTime);
  await press(page, 'Enter');
  await page.waitForTimeout(600);
  const s1 = await page.evaluate(() => document.querySelector("video").currentTime);
  check(s1 > s0 + 5, `OK skips the intro (${s0.toFixed(1)} → ${s1.toFixed(1)})`);
  await press(page, 'ArrowUp');
  await page.screenshot({ path: join(out, 'tv-07-player.png') });
  const before = await page.evaluate(() => document.querySelector('video').currentTime);
  await press(page, 'Escape'); // hide chrome
  await page.waitForTimeout(300);
  await press(page, 'ArrowRight');
  await page.waitForTimeout(500);
  const after = await page.evaluate(() => document.querySelector('video').currentTime);
  check(after > before + 5, `right = +10 s (${before.toFixed(1)} → ${after.toFixed(1)})`);
  await press(page, 'Escape'); // the seek showed the controls: the first back hides them
  check(await page.evaluate(() => !document.querySelector('.player').classList.contains('show')), 'back hides the controls first');
  await press(page, 'Escape');
  await page.waitForSelector('.detail-head', { timeout: 10000 }).then(() => ok('second back leaves the player'), () => bad('back did not leave player'));
  // series detail: seasons + episodes
  const shows = await (await fetch(SERVER + '/api/search?q=Test%20Show', { headers: auth })).json();
  const series = shows.filter(s => s.kind === 'series')[0];
  if (series) {
    await page.evaluate(id => { location.hash = '#/item/' + id; }, series.id);
    await page.waitForSelector('.ep-row', { timeout: 10000 });
    check((await page.$$('.ep-row')).length > 0, 'series shows episodes');
    await page.screenshot({ path: join(out, 'tv-08-series.png') });
  }
  // back on home asks before exiting
  await page.evaluate(() => { location.hash = '#/home'; });
  await page.waitForSelector('.facade');
  await press(page, 'Escape');
  await page.waitForSelector('.layer', { timeout: 3000 }).then(() => ok('back on home asks to exit'), () => bad('no exit confirmation'));
  await page.screenshot({ path: join(out, 'tv-09-exit.png') });
  await press(page, 'Escape');
  await page.evaluate(() => { location.hash = '#/settings'; });
  await page.waitForSelector('.set-row');
  await page.screenshot({ path: join(out, 'tv-10-settings.png') });
  await page.evaluate(() => { location.hash = '#/library'; });
  await page.waitForSelector('.grid .card', { timeout: 10000 });
  await page.screenshot({ path: join(out, 'tv-11-library.png') });
  check(errors.length === 0, 'no page errors' + (errors.length ? ': ' + errors.join(' | ') : ''));
  await ctx.close();
}

// ---------------------------------------------------------------- phone
console.log('Phone (390×844, touch, http://localhost)');
{
  const srv = createServer((req, res) => {
    const p = join(dist, decodeURIComponent((req.url || '/').split('?')[0]).replace(/^\/$/, '/index.html'));
    if (!existsSync(p)) { res.writeHead(404); res.end(); return; }
    const types = { '.html': 'text/html', '.js': 'text/javascript', '.woff2': 'font/woff2', '.css': 'text/css' };
    res.writeHead(200, { 'Content-Type': types[extname(p)] || 'application/octet-stream' });
    res.end(readFileSync(p));
  }).listen(5181);
  const ctx = await browser.newContext({ viewport: { width: 390, height: 844 }, deviceScaleFactor: 2, hasTouch: true, isMobile: true });
  const page = await ctx.newPage();
  const errors = [];
  page.on('pageerror', e => errors.push(e.message));
  await page.goto('http://localhost:5181/index.html');
  await page.waitForSelector('input.addr');
  check(await page.evaluate(() => document.documentElement.className) === 'mode-mobile', 'mobile mode');
  await page.fill('input.addr', host);
  await page.tap('button[type=submit]');
  await page.waitForSelector('input[autocomplete=username]', { timeout: 15000 });
  await page.screenshot({ path: join(out, 'phone-01-login.png') });
  await page.fill('input[autocomplete=username]', USER);
  await page.fill('input[type=password]', PASS);
  await page.tap('button[type=submit]');
  await page.waitForSelector('.who', { timeout: 15000 });
  await page.tap('.who');
  await page.waitForSelector('.facade', { timeout: 15000 });
  await page.waitForTimeout(500);
  check(await page.$('.nav.tabs') !== null, 'bottom tab bar');
  await page.screenshot({ path: join(out, 'phone-02-home.png') });
  await page.tap('.nav.tabs a[href="#/library"]');
  await page.waitForSelector('.grid .card', { timeout: 10000 });
  await page.screenshot({ path: join(out, 'phone-03-library.png') });
  await page.tap('.grid .card');
  await page.waitForSelector('.detail-head');
  await page.waitForTimeout(400);
  await page.screenshot({ path: join(out, 'phone-04-detail.png') });
  await page.tap('.nav.tabs a[href="#/settings"]');
  await page.waitForSelector('.set-row');
  await page.waitForTimeout(400);
  await page.screenshot({ path: join(out, 'phone-05-settings.png'), fullPage: true });
  check((await page.$$('.dev-row')).length >= 2, 'devices listed (TV + phone)');
  await page.setViewportSize({ width: 844, height: 390 });
  await page.evaluate(() => { location.hash = '#/home'; });
  await page.waitForSelector('.facade');
  await page.waitForTimeout(400);
  await page.screenshot({ path: join(out, 'phone-06-landscape.png') });
  // away from home: the saved (home) address is dead, the server's other address works
  await page.setViewportSize({ width: 390, height: 844 });
  const moved = await page.evaluate(async real => {
    const list = JSON.parse(localStorage.getItem('kinora.servers'));
    list[0].urls = [real];
    list[0].url = 'http://127.0.0.1:9'; // nothing listens there
    localStorage.setItem('kinora.servers', JSON.stringify(list));
    return list[0].id;
  }, SERVER);
  await page.evaluate(() => { location.hash = '#/start'; location.reload(); });
  await page.waitForSelector('.facade', { timeout: 30000 }).then(() => ok('dead home address → switched to the other address'), () => bad('no failover to the working address'));
  const now = await page.evaluate(id => JSON.parse(localStorage.getItem('kinora.servers')).filter(x => x.id === id)[0].url, moved);
  check(now === SERVER, 'working address remembered: ' + now);
  check(errors.length === 0, 'no page errors' + (errors.length ? ': ' + errors.join(' | ') : ''));
  await ctx.close();
  srv.close();
}

await browser.close();
console.log(failures ? `\x1b[31m${failures} FAILED\x1b[0m` : '\x1b[32mAPP E2E OK\x1b[0m');
process.exit(failures ? 1 : 0);
