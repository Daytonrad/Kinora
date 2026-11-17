// Generates every app icon, TV banner and splash image from one source logo.
//   npm run brand                                         (default Kinora mark)
//   npm run brand -- --logo sajat.png --name "Máté Mozi"  (own logo + app name)
//   npm run brand -- --logo sajat.png --banner szeles.png --bg "#101010"
// --logo    square PNG/SVG, at least 1024×1024 (full-bleed: it is the icon as-is)
// --banner  optional 16:9 image for the Android TV banner (default: logo on the background)
// --name    app name shown under the icon (Android, iOS, Tizen, webOS, PWA)
// --bg      background colour of splash screens and adaptive icons (default #141210)
import sharp from 'sharp';
import { existsSync, mkdirSync, readFileSync, writeFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

const here = dirname(fileURLToPath(import.meta.url));
const apps = join(here, '..');
const repo = join(apps, '..');

const args = process.argv.slice(2);
const opt = (k, d) => { const i = args.indexOf('--' + k); return i >= 0 && args[i + 1] ? args[i + 1] : d; };
const logo = opt('logo', join(here, 'logo.svg'));
const bannerSrc = opt('banner', existsSync(join(here, 'banner.svg')) && !args.includes('--logo') ? join(here, 'banner.svg') : '');
const name = opt('name', '');
const bg = opt('bg', '#141210');
if (!/^#[0-9a-fA-F]{6}$/.test(bg)) throw new Error('--bg must be #rrggbb');
if (!existsSync(logo)) throw new Error('logo not found: ' + logo);

const meta = await sharp(logo, { density: 512 }).metadata();
if (!/svg/.test(meta.format) && (meta.width < 1024 || meta.height < 1024)) throw new Error(`the logo must be at least 1024×1024 (got ${meta.width}×${meta.height})`);
if (Math.abs((meta.width || 1) / (meta.height || 1) - 1) > 0.02) throw new Error('the logo must be square');
const src = await sharp(logo, { density: 512 }).resize(1024, 1024, { fit: 'contain', background: bg }).png().toBuffer();

const written = [];
async function out(path, img) {
  mkdirSync(dirname(path), { recursive: true });
  await img.png({ compressionLevel: 9 }).toFile(path);
  written.push(path.slice(repo.length + 1).replace(/\\/g, '/'));
}
const icon = (size) => sharp(src).resize(size, size);
/** logo scaled to `frac` of a canvas, centred on the background colour */
async function onCanvas(w, h, frac) {
  const s = Math.round(Math.min(w, h) * frac);
  const l = await sharp(src).resize(s, s).toBuffer();
  return sharp({ create: { width: w, height: h, channels: 4, background: bg } }).composite([{ input: l, gravity: 'center' }]);
}
function roundMask(size) {
  return Buffer.from(`<svg width="${size}" height="${size}"><circle cx="${size / 2}" cy="${size / 2}" r="${size / 2}"/></svg>`);
}

// ---------------------------------------------------------------- Android (phone + TV)
const res = join(apps, 'android', 'android', 'app', 'src', 'main', 'res');
if (existsSync(res)) {
  const dens = { mdpi: 1, hdpi: 1.5, xhdpi: 2, xxhdpi: 3, xxxhdpi: 4 };
  for (const [d, f] of Object.entries(dens)) {
    await out(join(res, `mipmap-${d}`, 'ic_launcher.png'), icon(Math.round(48 * f)));
    await out(join(res, `mipmap-${d}`, 'ic_launcher_round.png'),
      sharp(await icon(Math.round(48 * f)).toBuffer()).composite([{ input: roundMask(Math.round(48 * f)), blend: 'dest-in' }]));
    // adaptive foreground: 108 dp canvas, the visible safe zone is the inner 66 dp
    await out(join(res, `mipmap-${d}`, 'ic_launcher_foreground.png'), await onCanvas(Math.round(108 * f), Math.round(108 * f), 0.6));
    await out(join(res, `drawable-port-${d}`, 'splash.png'), await onCanvas(Math.round(320 * f), Math.round(480 * f), 0.3));
    await out(join(res, `drawable-land-${d}`, 'splash.png'), await onCanvas(Math.round(480 * f), Math.round(320 * f), 0.3));
  }
  await out(join(res, 'drawable', 'splash.png'), await onCanvas(480, 320, 0.3));
  // Android TV launcher banner: 320×180 (xhdpi)
  const banner = bannerSrc ? sharp(bannerSrc, { density: 300 }).resize(320, 180, { fit: 'cover' }) : await onCanvas(320, 180, 0.7);
  await out(join(res, 'drawable', 'banner.png'), banner);
  const bgXml = join(res, 'values', 'ic_launcher_background.xml');
  writeFileSync(bgXml, `<?xml version="1.0" encoding="utf-8"?>\n<resources>\n    <color name="ic_launcher_background">${bg}</color>\n</resources>\n`);
  if (name) {
    const strings = join(res, 'values', 'strings.xml');
    let x = readFileSync(strings, 'utf8');
    const esc = name.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/'/g, "\\'");
    x = x.replace(/(<string name="app_name">)[^<]*/, `$1${esc}`).replace(/(<string name="title_activity_main">)[^<]*/, `$1${esc}`);
    writeFileSync(strings, x);
  }
}
if (name) {
  for (const p of ['android', 'ios']) {
    const f = join(apps, p, 'capacitor.config.json');
    if (existsSync(f)) { const j = JSON.parse(readFileSync(f, 'utf8')); j.appName = name; writeFileSync(f, JSON.stringify(j, null, 2) + '\n'); }
  }
}

// ---------------------------------------------------------------- iOS
// native SwiftUI app: a single 1024 px universal icon, iOS rounds the corners itself.
// Note: apps/ios/Kinora/AppIcon.icon (layered Icon Composer file, same name) takes
// precedence when it exists; delete it to ship this flat PNG instead.
const xc = join(apps, 'ios', 'Kinora', 'Assets.xcassets');
if (existsSync(xc)) {
  // iOS app icons may not have transparency
  await out(join(xc, 'AppIcon.appiconset', 'AppIcon.png'), sharp(src).flatten({ background: bg }).removeAlpha().resize(1024, 1024));
}

// ---------------------------------------------------------------- Apple TV
// tvOS wants a layered (parallax) icon and Top Shelf images: the back layer is the
// background, the front layer the mark alone, so the focus tilt lifts it off.
const tvxc = join(apps, 'ios', 'KinoraTV', 'Assets.xcassets');
if (existsSync(join(apps, 'ios', 'KinoraTV'))) {
  const brand = join(tvxc, 'App Icon & Top Shelf Image.brandassets');
  const json = (path, obj) => { mkdirSync(dirname(path), { recursive: true }); writeFileSync(path, JSON.stringify(obj, null, 2) + '\n'); };
  const info = { author: 'xcode', version: 1 };
  // Default mark: the amber K stroke without its square, so it floats over the back layer.
  const mark = args.includes('--logo') ? null : Buffer.from(
    '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 32 32" width="1024" height="1024">'
    + '<path d="M10 7v18M10 16l10-9M13 14l8 11" stroke="#f0b44c" stroke-width="3.4" fill="none" stroke-linecap="round"/></svg>');
  const front = async (w, h) => {
    const s = Math.round(h * (mark ? 0.78 : 0.6));
    const l = await sharp(mark ?? src, { density: 512 }).resize(s, s).png().toBuffer();
    return sharp({ create: { width: w, height: h, channels: 4, background: { r: 0, g: 0, b: 0, alpha: 0 } } }).composite([{ input: l, gravity: 'center' }]);
  };
  const back = (w, h) => sharp(Buffer.from(
    `<svg xmlns="http://www.w3.org/2000/svg" width="${w}" height="${h}"><defs><radialGradient id="g" cx="50%" cy="40%" r="75%">`
    + `<stop offset="0" stop-color="#2b2520"/><stop offset="1" stop-color="${bg}"/></radialGradient></defs>`
    + `<rect width="${w}" height="${h}" fill="${args.includes('--bg') ? bg : 'url(#g)'}"/></svg>`)).flatten({ background: bg }).removeAlpha();
  const stack = async (name, w, h, scales) => {
    const dir = join(brand, `${name}.imagestack`);
    for (const [layer, make] of [['Front', front], ['Back', back]]) {
      const set = join(dir, `${layer}.imagestacklayer`, 'Content.imageset');
      const images = [];
      for (const sc of scales) {
        const file = `${layer.toLowerCase()}${sc > 1 ? `@${sc}x` : ''}.png`;
        await out(join(set, file), await make(w * sc, h * sc));
        images.push({ filename: file, idiom: 'tv', scale: `${sc}x` });
      }
      json(join(set, 'Contents.json'), { images, info });
      json(join(dir, `${layer}.imagestacklayer`, 'Contents.json'), { info });
    }
    json(join(dir, 'Contents.json'), { info, layers: [{ filename: 'Front.imagestacklayer' }, { filename: 'Back.imagestacklayer' }] });
  };
  const shelf = async (name, w, h) => {
    const set = join(brand, `${name}.imageset`);
    const images = [];
    for (const sc of [1, 2]) {
      const file = `shelf${sc > 1 ? `@${sc}x` : ''}.png`;
      const img = bannerSrc
        ? sharp(await sharp(bannerSrc, { density: 300 * sc }).resize(Math.round(h * sc * 16 / 9), h * sc).png().toBuffer())
          .resize(w * sc, h * sc, { fit: 'contain', background: bg }).flatten({ background: bg })
        : await onCanvas(w * sc, h * sc, 0.5);
      await out(join(set, file), img);
      images.push({ filename: file, idiom: 'tv', scale: `${sc}x` });
    }
    json(join(set, 'Contents.json'), { images, info });
  };
  await stack('App Icon', 400, 240, [1, 2]);
  await stack('App Icon - App Store', 1280, 768, [1]);
  await shelf('Top Shelf Image', 1920, 720);
  await shelf('Top Shelf Image Wide', 2320, 720);
  json(join(brand, 'Contents.json'), {
    assets: [
      { filename: 'App Icon - App Store.imagestack', idiom: 'tv', role: 'primary-app-icon', size: '1280x768' },
      { filename: 'App Icon.imagestack', idiom: 'tv', role: 'primary-app-icon', size: '400x240' },
      { filename: 'Top Shelf Image Wide.imageset', idiom: 'tv', role: 'top-shelf-image-wide', size: '2320x720' },
      { filename: 'Top Shelf Image.imageset', idiom: 'tv', role: 'top-shelf-image', size: '1920x720' },
    ],
    info,
  });
}

// ---------------------------------------------------------------- Samsung Tizen & LG webOS
await out(join(apps, 'tizen', 'icon.png'), icon(512));
await out(join(apps, 'webos', 'icon.png'), icon(80));
await out(join(apps, 'webos', 'largeIcon.png'), icon(130));
await out(join(apps, 'webos', 'splash.png'), await onCanvas(1920, 1080, 0.22));
if (name) {
  const cfg = join(apps, 'tizen', 'config.xml');
  if (existsSync(cfg)) writeFileSync(cfg, readFileSync(cfg, 'utf8').replace(/<name>[^<]*<\/name>/, `<name>${name.replace(/&/g, '&amp;').replace(/</g, '&lt;')}</name>`));
  const ai = join(apps, 'webos', 'appinfo.json');
  if (existsSync(ai)) { const j = JSON.parse(readFileSync(ai, 'utf8')); j.title = name; writeFileSync(ai, JSON.stringify(j, null, 2) + '\n'); }
}

// ---------------------------------------------------------------- PWA (web client: iPhone "Add to Home Screen")
const pwa = join(repo, 'web', 'static', 'icons');
await out(join(pwa, 'icon-192.png'), icon(192));
await out(join(pwa, 'icon-512.png'), icon(512));
await out(join(pwa, 'maskable-512.png'), await onCanvas(512, 512, 0.7));
await out(join(pwa, 'apple-touch-icon.png'), sharp(src).flatten({ background: bg }).resize(180, 180));

console.log(`branding: ${written.length} images from ${logo}${name ? ` · name "${name}"` : ''}`);
for (const w of written) console.log('  ' + w);
