// Build helpers shared by the platform build scripts (no dependencies).
import { spawnSync } from 'node:child_process';
import { cpSync, rmSync, mkdirSync, readdirSync, statSync, readFileSync, writeFileSync, existsSync } from 'node:fs';
import { join, relative, sep, dirname } from 'node:path';
import { deflateRawSync, crc32 as zcrc32 } from 'node:zlib';
import { fileURLToPath } from 'node:url';

export const appsDir = join(dirname(fileURLToPath(import.meta.url)), '..');
export const sharedDist = join(appsDir, 'shared', 'dist');
export const isWin = process.platform === 'win32';

export function log(msg) { console.log('\x1b[33m›\x1b[0m ' + msg); }

/** Runs a command, streaming output; throws on failure. */
export function run(cmd, args, opts = {}) {
  log([cmd, ...args].join(' '));
  const r = spawnSync(cmd, args, { stdio: 'inherit', shell: isWin, ...opts });
  if (r.status !== 0) throw new Error(`${cmd} exited with ${r.status}`);
}

/** Whether a command exists on PATH. */
export function has(cmd) {
  const r = spawnSync(isWin ? 'where' : 'which', [cmd], { stdio: 'ignore', shell: isWin });
  return r.status === 0;
}

export function buildShared(extraEnv = {}) {
  run('npm', ['run', 'build', '-w', 'shared'], { cwd: appsDir, env: { ...process.env, ...extraEnv } });
}

export function fresh(dir) { rmSync(dir, { recursive: true, force: true }); mkdirSync(dir, { recursive: true }); }
export function copy(from, to) { cpSync(from, to, { recursive: true }); }

export function version() {
  return JSON.parse(readFileSync(join(appsDir, 'shared', 'package.json'), 'utf8')).version;
}

function walk(dir) {
  const out = [];
  for (const n of readdirSync(dir)) {
    const p = join(dir, n);
    if (statSync(p).isDirectory()) out.push(...walk(p)); else out.push(p);
  }
  return out;
}

const crc32 = zcrc32 || ((buf) => {
  let c, crc = 0xffffffff;
  for (let n = 0; n < buf.length; n++) {
    c = (crc ^ buf[n]) & 0xff;
    for (let k = 0; k < 8; k++) c = c & 1 ? 0xedb88320 ^ (c >>> 1) : c >>> 1;
    crc = (crc >>> 8) ^ c;
  }
  return (crc ^ 0xffffffff) >>> 0;
});

/** Writes a deflate zip of every file under dir (paths relative to dir, forward slashes). */
export function zipDir(dir, outFile, firstFiles = []) {
  let files = walk(dir).map(p => relative(dir, p).split(sep).join('/'));
  files = firstFiles.filter(f => files.includes(f)).concat(files.filter(f => !firstFiles.includes(f)).sort());
  const local = [], central = [];
  let offset = 0;
  for (const name of files) {
    const data = readFileSync(join(dir, name));
    const comp = deflateRawSync(data, { level: 9 });
    const nameBuf = Buffer.from(name, 'utf8');
    const crc = crc32(data) >>> 0;
    const h = Buffer.alloc(30);
    h.writeUInt32LE(0x04034b50, 0); h.writeUInt16LE(20, 4); h.writeUInt16LE(0x0800, 6); h.writeUInt16LE(8, 8);
    h.writeUInt16LE(0, 10); h.writeUInt16LE(0x21, 12); h.writeUInt32LE(crc, 14); h.writeUInt32LE(comp.length, 18);
    h.writeUInt32LE(data.length, 22); h.writeUInt16LE(nameBuf.length, 26); h.writeUInt16LE(0, 28);
    local.push(h, nameBuf, comp);
    const c = Buffer.alloc(46);
    c.writeUInt32LE(0x02014b50, 0); c.writeUInt16LE(20, 4); c.writeUInt16LE(20, 6); c.writeUInt16LE(0x0800, 8); c.writeUInt16LE(8, 10);
    c.writeUInt16LE(0, 12); c.writeUInt16LE(0x21, 14); c.writeUInt32LE(crc, 16); c.writeUInt32LE(comp.length, 20); c.writeUInt32LE(data.length, 24);
    c.writeUInt16LE(nameBuf.length, 28); c.writeUInt32LE(offset, 42);
    central.push(c, nameBuf);
    offset += 30 + nameBuf.length + comp.length;
  }
  const cdSize = central.reduce((n, b) => n + b.length, 0);
  const end = Buffer.alloc(22);
  end.writeUInt32LE(0x06054b50, 0); end.writeUInt16LE(files.length, 8); end.writeUInt16LE(files.length, 10);
  end.writeUInt32LE(cdSize, 12); end.writeUInt32LE(offset, 16);
  mkdirSync(dirname(outFile), { recursive: true });
  writeFileSync(outFile, Buffer.concat([...local, ...central, end]));
}

export { existsSync, join, readFileSync, writeFileSync, mkdirSync };
