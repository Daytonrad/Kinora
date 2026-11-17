// Server address parsing (pure: unit-tested in test/unit.test.mjs).

/**
 * Turns user input into candidate base URLs, in the order to try:
 * "192.168.1.10" → http://192.168.1.10:8096; "nas.local:9000" → http://nas.local:9000;
 * "http://10.0.0.5" → http://10.0.0.5:8096, then :80; "https://x.example" → https://x.example
 * (reverse proxy on 443) first, then :8096. Without a scheme http is tried first, then https.
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
  const out: string[] = [];
  if (hasPort) {
    out.push('http://' + hostPort + path, 'https://' + hostPort + path);
  } else {
    out.push('http://' + hostPort + ':8096' + path, 'http://' + hostPort + path, 'https://' + hostPort + path);
  }
  return out;
}
