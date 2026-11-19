// Builds the Titan OS app into apps/titan/dist/: a static site, not a package.
// Titan OS apps are web pages the TV opens from a fixed https address registered in the Titan OS
// developer portal. Host dist/ there (e.g. GitHub Pages); dist/kinora-titan.zip is the same for upload.
// The page only picks the server; the UI is the server's own web client in app mode. An https page
// cannot read an http server (mixed content), so for a home address it opens the server without
// checking it first, and /api/status allows any origin so https servers are still checked.
import { buildLauncher, zipDir, log, join } from '../scripts/lib.mjs';
import { dirname } from 'node:path';
import { fileURLToPath } from 'node:url';

const here = dirname(fileURLToPath(import.meta.url));
const site = join(here, 'dist', 'site');
buildLauncher(site, { platform: 'titan' });
const zip = join(here, 'dist', 'kinora-titan.zip');
zipDir(site, zip, ['index.html']);
log('Titan OS site: ' + site);
log('zip: ' + zip);
