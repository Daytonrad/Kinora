// Server address parsing (pure: unit-tested in test/unit.test.mjs).

/** True for IP literals, localhost and mDNS names: these live on the home network. */
export function isLocalHost(host: string): boolean {
  const h = host.replace(/^\[|\]$/g, '').toLowerCase();
  return /^\d{1,3}(\.\d{1,3}){3}$/.test(h) || h.indexOf(':') >= 0 || h === 'localhost' || /\.(local|lan|home|internal)$/.test(h) || h.indexOf('.') < 0;
}

/**
 * Turns user input into candidate base URLs, in the order to try:
 * "192.168.1.10" → http://192.168.1.10:8096, then :80, then https;
 * "kinora.pelda.hu" (a public name) → https://kinora.pelda.hu first (reverse proxy,
 * Cloudflare), then http on 8096 and 80; "nas.local:9000" → http, then https;
 * "http://10.0.0.5" → http://10.0.0.5:8096, then :80; "https://x.example" → https://x.example
 * (reverse proxy on 443) first, then :8096.
 */
export function candidateURLs(input: string): string[] {
  let s = (input || '').trim().replace(/\s+/g, '');
  if (!s) return [];
  s = s.replace(/\/+$/, '');
  const m = /^(https?):\/\/(.+)$/i.exec(s);
  const scheme = m ? m[1].toLowerCase() : '';
  const rest = m ? m[2] : s;
  const hostPort = rest.split('/')[0];
  const path = rest.slice(hostPort.length);
  const hasPort = /^\[.*\]:\d+$/.test(hostPort) || (/:\d+$/.test(hostPort) && hostPort.split(':').length === 2);
  if (scheme) {
    if (hasPort) return [scheme + '://' + rest];
    return scheme === 'https'
      ? ['https://' + rest, 'https://' + hostPort + ':8096' + path]
      : ['http://' + hostPort + ':8096' + path, 'http://' + rest];
  }
  const host = hasPort ? hostPort.replace(/:\d+$/, '') : hostPort;
  if (hasPort) {
    return isLocalHost(host)
      ? ['http://' + hostPort + path, 'https://' + hostPort + path]
      : ['https://' + hostPort + path, 'http://' + hostPort + path];
  }
  return isLocalHost(host)
    ? ['http://' + hostPort + ':8096' + path, 'http://' + hostPort + path, 'https://' + hostPort + path]
    : ['https://' + hostPort + path, 'http://' + hostPort + ':8096' + path, 'http://' + hostPort + path];
}

/** The base URL a response really came from (after redirects, e.g. http → https). */
export function baseFromResponse(responseURL: string, fallback: string): string {
  const i = responseURL ? responseURL.indexOf('/api/status') : -1;
  return i > 0 ? responseURL.slice(0, i) : fallback;
}

/** Merges addresses, most preferred first, without duplicates. */
export function mergeURLs(...lists: (string | undefined | null | string[])[]): string[] {
  const out: string[] = [];
  for (const l of lists) {
    for (const u of Array.isArray(l) ? l : [l]) {
      const v = (u || '').trim().replace(/\/+$/, '');
      if (v && out.indexOf(v) < 0) out.push(v);
    }
  }
  return out.slice(0, 8);
}
