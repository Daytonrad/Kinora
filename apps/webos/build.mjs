// Builds the LG webOS TV app into apps/webos/dist/kinora_<ver>_all.ipk.
//   node build.mjs             package with ares-package (@webos-tools/cli, a local devDependency)
//   node build.mjs --install   package, then ares-install + ares-launch on the device "tv"
//                              (register it once: npx ares-setup-device, see docs/TESZTELES_MA.md)
// webOS needs no signing for Developer Mode installs.
import { run, buildShared, buildLauncher, copy, sharedDist, version, log, join, readFileSync, writeFileSync, mkdirSync, existsSync, appsDir } from '../scripts/lib.mjs';
import { copyFileSync, readdirSync, rmSync } from 'node:fs';
import { dirname } from 'node:path';
import { fileURLToPath } from 'node:url';

const here = dirname(fileURLToPath(import.meta.url));
const www = join(here, 'www');
const dist = join(here, 'dist');
const ver = version();
const device = process.env.KINORA_WEBOS_DEVICE || 'tv';

// The app is the launcher: it finds the server and opens the server's web client (app mode).
buildLauncher(www, { platform: 'webos', legacy: true });
// webOS 4.x (Chromium 53) cannot run the web client: the launcher hands it the older bundled app in legacy/.
buildShared({ KINORA_PLATFORM: 'webos' });
copy(sharedDist, join(www, 'legacy'));
const info = JSON.parse(readFileSync(join(here, 'appinfo.json'), 'utf8'));
info.version = ver;
writeFileSync(join(www, 'appinfo.json'), JSON.stringify(info, null, 2));
for (const f of ['icon.png', 'largeIcon.png', 'splash.png']) copyFileSync(join(here, f), join(www, f));

mkdirSync(dist, { recursive: true });
for (const f of readdirSync(dist)) if (f.endsWith('.ipk')) rmSync(join(dist, f));
const bin = (n) => {
  const local = join(appsDir, 'node_modules', '.bin', n + (process.platform === 'win32' ? '.cmd' : ''));
  return existsSync(local) ? local : n;
};
run(bin('ares-package'), [www, '-o', dist, '--no-minify']);
const made = readdirSync(dist).filter(f => f.endsWith('.ipk'))[0];
const ipk = join(dist, `kinora_${ver}_all.ipk`);
if (made !== `kinora_${ver}_all.ipk`) { copyFileSync(join(dist, made), ipk); rmSync(join(dist, made)); }
log('IPK: ' + ipk);

if (process.argv.includes('--install')) {
  run(bin('ares-install'), ['-d', device, ipk]);
  run(bin('ares-launch'), ['-d', device, info.id]);
}
