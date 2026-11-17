// Builds the Samsung Tizen TV app into apps/tizen/dist/kinora.wgt.
//   node build.mjs             package (signed with the Tizen CLI when available)
//   node build.mjs --install   package, then install + launch on the TV (KINORA_TV_IP)
// Signing: Samsung TVs only install widgets signed with a Samsung certificate made in
// Tizen Studio's Certificate Manager. Name the security profile with KINORA_TIZEN_PROFILE
// (default "kinora"). Without the Tizen CLI an unsigned kinora-unsigned.wgt is written,
// which `tizen package -t wgt -s <profile> -- kinora-unsigned.wgt` can sign later.
import { run, has, buildShared, fresh, copy, sharedDist, version, log, zipDir, existsSync, join, readFileSync, writeFileSync, mkdirSync } from '../scripts/lib.mjs';
import { copyFileSync, readdirSync, rmSync } from 'node:fs';
import { dirname } from 'node:path';
import { fileURLToPath } from 'node:url';

const here = dirname(fileURLToPath(import.meta.url));
const www = join(here, 'www');
const dist = join(here, 'dist');
const ver = version();

buildShared({ KINORA_PLATFORM: 'tizen' });
fresh(www);
copy(sharedDist, www);
// Samsung's webapis (product info, screensaver, AVInfo/HDR) are injected by the TV at $WEBAPIS.
const idx = join(www, 'index.html');
writeFileSync(idx, readFileSync(idx, 'utf8').replace('<script defer src="./app.js">', '<script src="$WEBAPIS/webapis/webapis.js"></script>\n  <script defer src="./app.js">'));
writeFileSync(join(www, 'config.xml'), readFileSync(join(here, 'config.xml'), 'utf8').replace(/version="[^"]*"(\s+viewmodes)/, `version="${ver}"$1`));
copyFileSync(join(here, 'icon.png'), join(www, 'icon.png'));
mkdirSync(dist, { recursive: true });

function tizenCli() {
  if (has('tizen')) return 'tizen';
  const roots = [process.env.TIZEN_STUDIO, 'C:/tizen-studio', join(process.env.USERPROFILE || '', 'tizen-studio'), join(process.env.HOME || '', 'tizen-studio')].filter(Boolean);
  for (const r of roots) {
    for (const p of [join(r, 'tools', 'ide', 'bin', 'tizen.bat'), join(r, 'tools', 'ide', 'bin', 'tizen')]) if (existsSync(p)) return p;
  }
  return null;
}
const cli = tizenCli();
let wgt;
if (cli) {
  const profile = process.env.KINORA_TIZEN_PROFILE || 'kinora';
  for (const f of readdirSync(www)) if (f.endsWith('.wgt')) rmSync(join(www, f));
  run(cli, ['package', '-t', 'wgt', '-s', profile, '--', www]);
  const made = readdirSync(www).filter(f => f.endsWith('.wgt'))[0];
  wgt = join(dist, 'kinora.wgt');
  copyFileSync(join(www, made), wgt);
  rmSync(join(www, made));
  log('signed widget: ' + wgt);
} else {
  wgt = join(dist, 'kinora-unsigned.wgt');
  zipDir(www, wgt, ['config.xml']);
  log('Tizen CLI not found → unsigned widget: ' + wgt);
  log('A TV-re telepítéshez aláírás kell (Tizen Studio + Samsung tanúsítvány) – lásd docs/TESZTELES_MA.md.');
}

if (process.argv.includes('--install')) {
  if (!cli || !wgt.endsWith('kinora.wgt')) throw new Error('installing needs the Tizen CLI and a signed widget');
  const sdb = cli.replace(/tools[\\/]ide[\\/]bin[\\/]tizen(\.bat)?$/, 'tools/sdb' + (process.platform === 'win32' ? '.exe' : ''));
  const ip = process.env.KINORA_TV_IP;
  if (!ip) throw new Error('set KINORA_TV_IP to the TV address');
  run(cli === 'tizen' ? 'sdb' : sdb, ['connect', ip]);
  const target = process.env.KINORA_TV_SERIAL || `${ip}:26101`;
  run(cli, ['install', '-n', wgt, '-t', target]);
  run(cli, ['run', '-p', 'KinoraTV01.Kinora', '-t', target]);
}
void ver;
