// Unit tests for the pure parts of the app (address parsing, WebVTT, capability table).
// esbuild (shipped with vite) bundles the TypeScript modules, node:test runs them.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { build } from 'esbuild';
import { mkdtempSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join, dirname } from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';

const src = join(dirname(fileURLToPath(import.meta.url)), '..', 'src');
async function load(entry) {
  const r = await build({ entryPoints: [join(src, entry)], bundle: true, format: 'esm', platform: 'neutral', write: false, logLevel: 'silent' });
  const f = join(mkdtempSync(join(tmpdir(), 'kinora-')), 'm.mjs');
  writeFileSync(f, r.outputFiles[0].text);
  return import(pathToFileURL(f).href);
}

const { candidateURLs, baseFromResponse, mergeURLs, isLocalHost } = await load('address.ts');
const { parseVTT, activeCues } = await load('player/subs.ts');
const { PLATFORM_CAPS, mergeCaps } = await load('player/caps.ts');

test('IP only → http with port 8096 first', () => {
  assert.deepEqual(candidateURLs('192.168.1.10'), ['http://192.168.1.10:8096', 'http://192.168.1.10', 'https://192.168.1.10']);
});
test('IP:port keeps the port, http first', () => {
  assert.deepEqual(candidateURLs(' 192.168.1.10:9000 '), ['http://192.168.1.10:9000', 'https://192.168.1.10:9000']);
});
test('http:// without port tries 8096 then 80', () => {
  assert.deepEqual(candidateURLs('http://10.0.0.5'), ['http://10.0.0.5:8096', 'http://10.0.0.5']);
});
test('https:// without port tries 443 (proxy) then 8096', () => {
  assert.deepEqual(candidateURLs('https://kinora.pelda.hu/'), ['https://kinora.pelda.hu', 'https://kinora.pelda.hu:8096']);
});
test('scheme + port is used as given; hostnames work', () => {
  assert.deepEqual(candidateURLs('HTTP://nas.local:8096'), ['http://nas.local:8096']);
  assert.equal(candidateURLs('nas.local')[0], 'http://nas.local:8096');
  assert.deepEqual(candidateURLs(''), []);
});

test('public names try https first (reverse proxy / Cloudflare), home names http', () => {
  assert.deepEqual(candidateURLs('kinora.pelda.hu'), ['https://kinora.pelda.hu', 'http://kinora.pelda.hu:8096', 'http://kinora.pelda.hu']);
  assert.deepEqual(candidateURLs('mozi.duckdns.org:8920'), ['https://mozi.duckdns.org:8920', 'http://mozi.duckdns.org:8920']);
  assert.equal(candidateURLs('nas')[0], 'http://nas:8096');
  assert.ok(isLocalHost('192.168.1.10') && isLocalHost('nas.local') && isLocalHost('nas') && !isLocalHost('kinora.pelda.hu'));
});
test('the address after a redirect is kept, addresses merge without duplicates', () => {
  assert.equal(baseFromResponse('https://kinora.pelda.hu/api/status', 'http://kinora.pelda.hu'), 'https://kinora.pelda.hu');
  assert.equal(baseFromResponse('', 'http://10.0.0.5:8096'), 'http://10.0.0.5:8096');
  assert.deepEqual(mergeURLs('http://192.168.1.10:8096', ['https://kinora.pelda.hu/', 'http://192.168.1.10:8096'], undefined, ''),
    ['http://192.168.1.10:8096', 'https://kinora.pelda.hu']);
});

test('WebVTT parsing: timing, tags, multi-line, comma decimals', () => {
  const cues = parseVTT('WEBVTT\n\n1\n00:00:01.000 --> 00:00:03.500 line:90%\n<i>Árvíztűrő</i>\ntükörfúrógép\n\n00:01:02,250 --> 00:01:04.000\nB &amp; C\n');
  assert.equal(cues.length, 2);
  assert.deepEqual(cues[0], { start: 1, end: 3.5, text: 'Árvíztűrő\ntükörfúrógép' });
  assert.equal(cues[1].start, 62.25);
  assert.equal(cues[1].text, 'B & C');
  assert.equal(activeCues(cues, 2)[0].start, 1);
  assert.equal(activeCues(cues, 10).length, 0);
  assert.equal(activeCues(cues, 63).length, 1);
});
test('VTT character references decode once', () => {
  const cues = parseVTT('WEBVTT\r\n\r\n00:00:01.000 --> 00:00:02.000\r\n&amp;lt;b&amp;gt; &lt;3 &#39;x&#x27; &#x1F600; &bogus; & a\r\n');
  assert.equal(cues[0].text, "&lt;b&gt; <3 'x' \u{1F600} &bogus; & a");
});

test('platform table: TVs direct-play MKV/HEVC/AC-3, phones rely on probing', () => {
  for (const tv of ['tizen', 'webos']) {
    const c = PLATFORM_CAPS[tv];
    for (const v of ['h264', 'hevc']) assert.ok(c.video.includes(v), `${tv} ${v}`);
    for (const a of ['aac', 'ac3', 'eac3']) assert.ok(c.audio.includes(a), `${tv} ${a}`);
    for (const k of ['mp4', 'mkv', 'ts']) assert.ok(c.containers.includes(k), `${tv} ${k}`);
  }
  assert.ok(PLATFORM_CAPS.ios.video.includes('hevc'));
  assert.ok(!PLATFORM_CAPS.android.containers.includes('mkv'), 'Android WebView does not claim MKV');
  assert.equal(PLATFORM_CAPS.web.video.length, 0, 'the browser reports only what it probes');
});

test('mergeCaps: union without duplicates, HDR only with HEVC', () => {
  const probed = { video: ['h264', 'vp9'], audio: ['aac'], containers: ['mp4', 'webm'], hdr: false };
  const m = mergeCaps(probed, 'tizen', true);
  assert.deepEqual(m.video, ['h264', 'vp9', 'hevc', 'mpeg4']);
  assert.ok(m.containers.includes('mkv'));
  assert.equal(new Set(m.audio).size, m.audio.length);
  assert.equal(m.hdr, true);
  assert.equal(mergeCaps(probed, 'android', true).hdr, false, 'no HEVC → no HDR claim');
});
