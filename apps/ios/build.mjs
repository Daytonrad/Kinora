// Prepares the iOS Xcode project (apps/ios/ios/App) and, on macOS, builds it.
//   node build.mjs              build shared UI, copy it in, `cap sync ios` (works on Windows too)
//   node build.mjs --simulator  + unsigned simulator build with xcodebuild (macOS + Xcode; used by CI)
// Installing on an iPhone needs a Mac with Xcode: open ios/App/App.xcodeproj, pick your
// Apple ID team under Signing & Capabilities, connect the phone, Run. See apps/README.md.
import { run, buildShared, fresh, copy, sharedDist, version, log, join } from '../scripts/lib.mjs';
import { dirname } from 'node:path';
import { fileURLToPath } from 'node:url';

const here = dirname(fileURLToPath(import.meta.url));
buildShared({ KINORA_PLATFORM: 'ios' });
fresh(join(here, 'www'));
copy(sharedDist, join(here, 'www'));
run('npx', ['cap', 'sync', 'ios'], { cwd: here });

if (process.argv.includes('--simulator')) {
  if (process.platform !== 'darwin') throw new Error('iOS builds need macOS + Xcode');
  run('xcodebuild', ['-project', 'ios/App/App.xcodeproj', '-scheme', 'App', '-configuration', 'Debug',
    '-sdk', 'iphonesimulator', '-destination', 'generic/platform=iOS Simulator', '-derivedDataPath', 'build',
    `MARKETING_VERSION=${version()}`, 'CODE_SIGNING_ALLOWED=NO', 'build'], { cwd: here, shell: false });
  log('simulator build OK: apps/ios/build/Build/Products/Debug-iphonesimulator/App.app');
} else {
  log('Xcode project ready: apps/ios/ios/App/App.xcodeproj (open it on a Mac)');
}
