// Saved servers, the API client and address handling.
import { T } from './i18n';
import { candidateURLs } from './address';
export { candidateURLs };
import { storeGet, storeSet, getPlatform, APP_VERSION, deviceName, MIN_SERVER_API, CLIENT_API } from './platform';

export interface ServerStatus {
  product?: string; version: string; api_version?: number; min_client_api?: number; server_id?: string;
  server_name: string; logo_url?: string; quickconnect_enabled?: boolean; setup_needed?: boolean;
}

export interface SavedServer {
  id: string;          // server_id (or the address when the server is older)
  url: string;         // normalized base URL, no trailing slash
  name: string;
  token?: string;
  deviceId?: string;
  username?: string;
  lastUsed: number;
}

// ------------------------------------------------------------------ address parsing

export class HTTPError extends Error {
  constructor(public status: number, msg: string, public body?: any) { super(msg); }
}

function timeoutFetch(url: string, init: RequestInit, ms: number): Promise<Response> {
  return new Promise((resolve, reject) => {
    const t = window.setTimeout(() => reject(new Error('timeout')), ms);
    fetch(url, init).then(r => { window.clearTimeout(t); resolve(r); }, e => { window.clearTimeout(t); reject(e); });
  });
}

/** Asks base/api/status; resolves only if it is a Kinora server. */
export async function probe(base: string, ms = 4000): Promise<ServerStatus> {
  const r = await timeoutFetch(base + '/api/status', { method: 'GET', cache: 'no-store' }, ms);
  if (!r.ok) throw new Error('status ' + r.status);
  const j = await r.json();
  if (!j || (j.product !== 'kinora' && typeof j.setup_needed !== 'boolean')) throw new Error('not kinora');
  return j as ServerStatus;
}

export type ConnectResult = { url: string; status: ServerStatus } | { error: string };

/** Tries every candidate URL for the input and returns the first Kinora server. */
export async function resolveServer(input: string): Promise<ConnectResult> {
  const cands = candidateURLs(input);
  if (!cands.length) return { error: T.unreachable(input) };
  let sawSomething = false;
  for (const url of cands) {
    try {
      const status = await probe(url);
      const compat = checkCompat(status);
      if (compat) return { error: compat };
      if (status.setup_needed) return { error: T.setupNeeded };
      return { url, status };
    } catch (e: any) {
      if (e && e.message === 'not kinora') sawSomething = true;
    }
  }
  return { error: sawSomething ? T.notKinora(input) : T.unreachable(input) };
}

/** Version handshake: null when compatible, else a Hungarian explanation. */
export function checkCompat(s: ServerStatus): string | null {
  const api = s.api_version || 1;
  if (api < MIN_SERVER_API) return T.serverTooOld;
  if ((s.min_client_api || 1) > CLIENT_API) return T.serverTooNew;
  return null;
}

// ------------------------------------------------------------------ saved servers

const KEY = 'kinora.servers';
const CUR = 'kinora.current';

export async function loadServers(): Promise<SavedServer[]> {
  try { return JSON.parse((await storeGet(KEY)) || '[]'); } catch { return []; }
}
export async function saveServers(list: SavedServer[]): Promise<void> { await storeSet(KEY, JSON.stringify(list)); }
export async function currentServerId(): Promise<string | null> { return storeGet(CUR); }
export async function setCurrentServer(id: string | null): Promise<void> { await storeSet(CUR, id); }

export async function upsertServer(s: SavedServer): Promise<void> {
  const list = (await loadServers()).filter(x => x.id !== s.id && x.url !== s.url);
  list.unshift(s);
  await saveServers(list.slice(0, 12));
  await setCurrentServer(s.id);
}

export async function removeServer(id: string): Promise<void> {
  await saveServers((await loadServers()).filter(x => x.id !== id));
  if ((await currentServerId()) === id) await setCurrentServer(null);
}

// ------------------------------------------------------------------ API client

export class Api {
  onUnauthorized: () => void = () => {};
  mediaToken = '';
  constructor(public server: SavedServer) {}

  get base(): string { return this.server.url; }

  async req<R = any>(method: string, path: string, body?: any, opts: { timeout?: number } = {}): Promise<R> {
    const headers: Record<string, string> = {};
    if (body !== undefined) headers['Content-Type'] = 'application/json';
    if (this.server.token) headers['Authorization'] = 'Bearer ' + this.server.token;
    let r: Response;
    try {
      r = await timeoutFetch(this.base + path, { method, headers, body: body !== undefined ? JSON.stringify(body) : undefined, cache: 'no-store' }, opts.timeout || 20000);
    } catch (e) {
      throw new HTTPError(0, T.netErr);
    }
    let data: any = null;
    const ct = r.headers.get('content-type') || '';
    if (ct.indexOf('json') >= 0) data = await r.json().catch(() => null);
    if (!r.ok) {
      if (r.status === 401 && this.server.token && path.indexOf('/api/quickconnect') !== 0) this.onUnauthorized();
      throw new HTTPError(r.status, (data && data.error) || 'HTTP ' + r.status, data);
    }
    return data as R;
  }
  get<R = any>(p: string) { return this.req<R>('GET', p); }
  post<R = any>(p: string, b: any = {}) { return this.req<R>('POST', p, b); }
  patch<R = any>(p: string, b: any = {}) { return this.req<R>('PATCH', p, b); }
  del<R = any>(p: string) { return this.req<R>('DELETE', p); }

  /** Absolute URL for a server-relative path (adds the base). */
  abs(path: string): string { return /^https?:/.test(path) ? path : this.base + path; }

  image(id: number, kind: 'poster' | 'backdrop' | 'thumb'): string {
    return `${this.base}/api/images/${id}/${kind}${this.mediaToken ? '?st=' + encodeURIComponent(this.mediaToken) : ''}`;
  }

  device() {
    return { device_name: deviceName(), platform: getPlatform(), app_version: APP_VERSION };
  }
}
