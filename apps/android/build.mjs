// Builds the Kinora APK (phone + Android TV) into apps/android/dist/kinora.apk.
//   node build.mjs              debug APK (installable right away)
//   node build.mjs --release    signed release APK (KINORA_KEYSTORE, KINORA_KEYSTORE_PASSWORD,
//                               KINORA_KEY_ALIAS, KINORA_KEY_PASSWORD from the environment)
//   node build.mjs --install    debug build, then `adb install -r` on the connected device
// Gradle runs locally when JAVA_HOME and ANDROID_HOME are set, otherwise inside the
// kinora-android-build Docker image (apps/android/docker/Dockerfile): nothing to install on the host.
import { run, has, buildLauncher, appsDir, version, isWin, log, existsSync, join, mkdirSync } from '../scripts/lib.mjs';
import { copyFileSync } from 'node:fs';
import { dirname } from 'node:path';
import { fileURLToPath } from 'node:url';

const here = dirname(fileURLToPath(import.meta.url));
const release = process.argv.includes('--release');
const install = process.argv.includes('--install');

// The app is the launcher: it finds the server and opens the server's web client in the WebView
// (app mode). Phone or Android TV is decided at run time (KinoraAndroid.isTv()).
buildLauncher(join(here, 'www'), { platform: 'android' });

if (!existsSync(join(here, 'android'))) throw new Error('apps/android/android is missing (it is committed; run `npx cap add android` only for a fresh checkout of the template)');
run('npx', ['cap', 'sync', 'android'], { cwd: here });

const task = release ? 'assembleRelease' : 'assembleDebug';
const env = { ...process.env, KINORA_VERSION: version() };
const local = process.env.JAVA_HOME && (process.env.ANDROID_HOME || process.env.ANDROID_SDK_ROOT);
if (local) {
  run(isWin ? 'gradlew.bat' : './gradlew', [task, '--no-daemon'], { cwd: join(here, 'android'), env });
} else {
  if (!has('docker')) throw new Error('Neither a local JDK + Android SDK (JAVA_HOME, ANDROID_HOME) nor Docker was found.');
  const img = 'kinora-android-build';
  try { run('docker', ['image', 'inspect', img], { stdio: 'ignore', shell: false }); }
  catch { run('docker', ['build', '-t', img, join(here, 'docker')], { shell: false }); }
  const mount = appsDir.replace(/\\/g, '/');
  const pass = ['KINORA_VERSION', 'KINORA_KEYSTORE', 'KINORA_KEYSTORE_PASSWORD', 'KINORA_KEY_ALIAS', 'KINORA_KEY_PASSWORD']
    .flatMap(k => (env[k] ? ['-e', `${k}=${k === 'KINORA_KEYSTORE' ? '/keystore/' + env[k].split(/[\\/]/).pop() : env[k]}`] : []));
  const ks = env.KINORA_KEYSTORE ? ['-v', `${dirname(env.KINORA_KEYSTORE).replace(/\\/g, '/')}:/keystore:ro`] : [];
  run('docker', ['run', '--rm', '-v', `${mount}:/apps`, '-v', 'kinora-gradle:/gradle', ...ks, ...pass, '-w', '/apps/android/android',
    img, 'bash', '-c', `chmod +x gradlew && ./gradlew ${task} --no-daemon -q`], { shell: false, env: { ...env, MSYS_NO_PATHCONV: '1' } });
}

const out = join(here, 'android', 'app', 'build', 'outputs', 'apk', release ? 'release' : 'debug', release ? 'app-release.apk' : 'app-debug.apk');
mkdirSync(join(here, 'dist'), { recursive: true });
const dest = join(here, 'dist', release ? 'kinora-release.apk' : 'kinora.apk');
copyFileSync(out, dest);
log('APK: ' + dest);

if (install) {
  if (!has('adb')) throw new Error('adb not found – install Android platform-tools (see docs/TESZTELES_MA.md) or copy the APK to the phone.');
  run('adb', ['install', '-r', dest]);
}
