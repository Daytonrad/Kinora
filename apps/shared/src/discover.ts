// "Szerver keresése": phones use the native UDP broadcast (KinoraNative.discover,
// port 7359). Web runtimes cannot send UDP, so TVs ask the platform for their own
// IP and probe <subnet>.1–254:8096/api/status over HTTP with a small worker pool.
import { probe } from './api';
import { KinoraNative, DiscoveredServer, getPlatform, isNative } from './platform';

const w = window as any;

async function nativeDiscover(): Promise<DiscoveredServer[]> {
  const r = await KinoraNative.discover({ timeoutMs: 2500, port: 7359 });
  return r.servers || [];
}

function tizenIP(): Promise<string | null> {
  return new Promise(resolve => {
    try {
      const sys = w.tizen.systeminfo;
      let done = false;
      const fin = (ip: string | null) => { if (!done) { done = true; resolve(ip); } };
      sys.getPropertyValue('ETHERNET_NETWORK', (n: any) => n && n.ipAddress ? fin(n.ipAddress) : null, () => null);
      sys.getPropertyValue('WIFI_NETWORK', (n: any) => n && n.ipAddress ? fin(n.ipAddress) : null, () => null);
      window.setTimeout(() => fin(null), 1500);
    } catch { resolve(null); }
  });
}

function webosIP(): Promise<string | null> {
  return new Promise(resolve => {
    try {
      w.webOS.service.request('luna://com.palm.connectionmanager', {
        method: 'getStatus', parameters: {},
        onSuccess: (r: any) => resolve((r.wired && r.wired.ipAddress) || (r.wifi && r.wifi.ipAddress) || null),
        onFailure: () => resolve(null),
      });
      window.setTimeout(() => resolve(null), 1500);
    } catch { resolve(null); }
  });
}

/** Best-effort local IPv4 via WebRTC (often hidden behind mDNS names in modern browsers). */
function rtcIP(): Promise<string | null> {
  return new Promise(resolve => {
    try {
      const pc = new (w.RTCPeerConnection || w.webkitRTCPeerConnection)({ iceServers: [] });
      pc.createDataChannel('x');
      pc.onicecandidate = (e: any) => {
        const m = e && e.candidate && /(\d+\.\d+\.\d+\.\d+)/.exec(e.candidate.candidate);
        if (m && !/^(0|127)\./.test(m[1])) { resolve(m[1]); pc.close(); }
      };
      pc.createOffer().then((o: any) => pc.setLocalDescription(o));
      window.setTimeout(() => { resolve(null); try { pc.close(); } catch { /* */ } }, 1200);
    } catch { resolve(null); }
  });
}

async function localIP(): Promise<string | null> {
  const p = getPlatform();
  if (p === 'tizen') return tizenIP();
  if (p === 'webos' && w.webOS && w.webOS.service) return webosIP();
  return rtcIP();
}

async function scanSubnets(prefixes: string[], onFound: (s: DiscoveredServer) => void): Promise<void> {
  const hosts: string[] = [];
  for (const pre of prefixes) for (let i = 1; i < 255; i++) hosts.push(pre + i);
  let next = 0;
  const worker = async () => {
    while (next < hosts.length) {
      const host = hosts[next++];
      const url = 'http://' + host + ':8096';
      try {
        const s = await probe(url, 900);
        onFound({ name: s.server_name, address: url, id: s.server_id || url, version: s.version });
      } catch { /* nothing there */ }
    }
  };
  const pool: Promise<void>[] = [];
  for (let i = 0; i < 32; i++) pool.push(worker());
  await Promise.all(pool);
}

/** Finds Kinora servers on the LAN; calls onFound as they appear, resolves when done. */
export async function discover(onFound: (s: DiscoveredServer) => void): Promise<void> {
  const seen: Record<string, boolean> = {};
  const emit = (s: DiscoveredServer) => {
    const k = s.id || s.address;
    if (!seen[k]) { seen[k] = true; onFound(s); }
  };
  if (isNative) {
    try { (await nativeDiscover()).forEach(emit); } catch { /* plugin missing */ }
    if (Object.keys(seen).length) return;
  }
  const ip = await localIP();
  const prefixes = ip ? [ip.replace(/\d+$/, '')] : ['192.168.1.', '192.168.0.'];
  await scanSubnets(prefixes, emit);
}
