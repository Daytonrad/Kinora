// Old TV engines cannot run ES modules and refuse crossorigin scripts from file://.
// Rewrite the built index.html to load the IIFE bundle as a classic deferred script.
import { readFileSync, writeFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

const dist = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');
const p = join(dist, 'index.html');
let html = readFileSync(p, 'utf8');
html = html
  .replace(/<script type="module" crossorigin src="([^"]+)"><\/script>/g, '<script defer src="$1"></script>')
  .replace(/<script type="module" src="([^"]+)"><\/script>/g, '<script defer src="$1"></script>')
  .replace(/ crossorigin/g, '');
writeFileSync(p, html);
if (/type="module"/.test(html)) throw new Error('index.html still has a module script');
console.log('postbuild: classic script index.html');
