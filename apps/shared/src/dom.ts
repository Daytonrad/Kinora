// Tiny DOM builder: h('div.a.b', {attr, onclick}, ...children). Text is always escaped.
export type Child = Node | string | number | null | undefined | false | Child[];
export type Props = Record<string, any> | null;

export function h<K extends keyof HTMLElementTagNameMap>(tag: K | string, props?: Props, ...kids: Child[]): HTMLElement {
  const [name, ...classes] = tag.split('.');
  const el = document.createElement(name || 'div');
  if (classes.length) el.className = classes.join(' ');
  if (props) {
    for (const k of Object.keys(props)) {
      const v = props[k];
      if (v === undefined || v === null || v === false) continue;
      if (k.slice(0, 2) === 'on' && typeof v === 'function') el.addEventListener(k.slice(2).toLowerCase(), v);
      else if (k === 'class') el.className = (el.className ? el.className + ' ' : '') + v;
      else if (k === 'style' && typeof v === 'object') Object.assign(el.style, v);
      else if (k === 'value' || k === 'checked' || k === 'selected' || k === 'disabled') (el as any)[k] = v;
      else el.setAttribute(k, v === true ? '' : String(v));
    }
  }
  add(el, kids);
  return el;
}

export function add(el: Node, kids: Child[]): Node {
  for (const k of kids) {
    if (k === null || k === undefined || k === false) continue;
    if (Array.isArray(k)) add(el, k);
    else if (k instanceof Node) el.appendChild(k);
    else el.appendChild(document.createTextNode(String(k)));
  }
  return el;
}

export function put(el: HTMLElement, ...kids: Child[]): HTMLElement {
  while (el.firstChild) el.removeChild(el.firstChild);
  add(el, kids);
  return el;
}

export const $ = <E extends Element = HTMLElement>(s: string, r: ParentNode = document) => r.querySelector(s) as E | null;
export const $$ = <E extends Element = HTMLElement>(s: string, r: ParentNode = document) => Array.prototype.slice.call(r.querySelectorAll(s)) as E[];

// 24-unit stroke icons, one weight (DESIGN.md: no emoji, drawn SVG only).
const PATHS: Record<string, string> = {
  play: 'M7 4.5v15l12-7.5z',
  pause: 'M7 4h3.5v16H7zM13.5 4H17v16h-3.5z',
  back: 'M15 5l-7 7 7 7',
  rew: 'M11 7L4 12l7 5zM20 7l-7 5 7 5z',
  fwd: 'M13 7l7 5-7 5zM4 7l7 5-7 5z',
  r10: 'M4 12a8 8 0 1 0 2.3-5.6M4 4v4h4',
  f10: 'M20 12a8 8 0 1 1-2.3-5.6M20 4v4h-4',
  sub: 'M3 6h18v12H3zM6 11h5M13 11h5M6 14h8',
  audio: 'M4 9v6h4l5 4V5L8 9zM16.5 8.5a5 5 0 0 1 0 7',
  gear: 'M12 9a3 3 0 1 0 0 6 3 3 0 0 0 0-6zM12 2v3M12 19v3M2 12h3M19 12h3M4.9 4.9L7 7M17 17l2.1 2.1M4.9 19.1L7 17M17 7l2.1-2.1',
  search: 'M10.5 4a6.5 6.5 0 1 0 0 13 6.5 6.5 0 0 0 0-13zM15.5 15.5L21 21',
  home: 'M3 11l9-7 9 7M5 9.5V20h5v-6h4v6h5V9.5',
  grid: 'M4 4h7v7H4zM13 4h7v7h-7zM4 13h7v7H4zM13 13h7v7h-7z',
  info: 'M12 3a9 9 0 1 0 0 18 9 9 0 0 0 0-18zM12 11v6M12 7.5v.5',
  check: 'M5 12.5l4.5 4.5L19 7.5',
  heart: 'M12 20s-7-4.5-7-10a4 4 0 0 1 7-2.6A4 4 0 0 1 19 10c0 5.5-7 10-7 10z',
  eye: 'M2 12s3.5-6.5 10-6.5S22 12 22 12s-3.5 6.5-10 6.5S2 12 2 12zM12 9.5a2.5 2.5 0 1 0 0 5 2.5 2.5 0 0 0 0-5z',
  hide: 'M3 3l18 18M10.6 6.1A9.8 9.8 0 0 1 12 6c6.5 0 10 6 10 6a17 17 0 0 1-3.2 3.9M6.3 7.6C3.6 9.4 2 12 2 12s3.5 6 10 6c1.6 0 3-.3 4.2-.9',
  star: 'M12 3.5l2.6 5.4 5.9.8-4.3 4.1 1 5.8L12 16.8l-5.2 2.8 1-5.8-4.3-4.1 5.9-.8z',
  wifi: 'M2 9a15 15 0 0 1 20 0M5.5 12.5a10 10 0 0 1 13 0M9 16a5 5 0 0 1 6 0M12 19.5v.5',
  plus: 'M12 5v14M5 12h14',
  x: 'M6 6l12 12M18 6L6 18',
  lock: 'M6 11h12v9H6zM8.5 11V8a3.5 3.5 0 0 1 7 0v3',
  qr: 'M4 4h6v6H4zM14 4h6v6h-6zM4 14h6v6H4zM14 14h2v2h-2zM18 14h2v2h-2zM14 18h2v2h-2zM18 18h2v2h-2z',
  key: 'M14.5 4a5.5 5.5 0 1 0 0 11 5.5 5.5 0 0 0 0-11zM10.5 13.5L3 21M5.5 18.5L8 21',
  skip: 'M5 5l9 7-9 7zM17 5v14',
  pip: 'M3 5h18v14H3zM12 12h7v5h-7z',
  tv: 'M3 6h18v11H3zM8 21h8M12 17v4',
  phone: 'M7 3h10v18H7zM11 18h2',
  server: 'M4 4h16v6H4zM4 14h16v6H4zM7.5 7h.5M7.5 17h.5',
  quality: 'M4 6h16v12H4zM8 10v4M8 12h3M11 10v4M14 10h2a2 2 0 0 1 0 4h-2z',
};

export function icon(name: string, filled = false): SVGSVGElement {
  const ns = 'http://www.w3.org/2000/svg';
  const svg = document.createElementNS(ns, 'svg');
  svg.setAttribute('viewBox', '0 0 24 24');
  svg.setAttribute('aria-hidden', 'true');
  svg.setAttribute('class', 'ico');
  const p = document.createElementNS(ns, 'path');
  p.setAttribute('d', PATHS[name] || '');
  if (filled || name === 'play' || name === 'pause') p.setAttribute('class', 'fill');
  svg.appendChild(p);
  return svg;
}

let toastTimer = 0;
export function toast(msg: string, err = false): void {
  const box = document.getElementById('toasts');
  if (!box) return;
  const el = h('div.toast' + (err ? '.err' : ''), null, msg);
  box.appendChild(el);
  window.clearTimeout(toastTimer);
  window.setTimeout(() => el.parentNode && el.parentNode.removeChild(el), err ? 6000 : 3200);
}

export function debounce<A extends any[]>(fn: (...a: A) => void, ms: number): (...a: A) => void {
  let t = 0;
  return (...a: A) => { window.clearTimeout(t); t = window.setTimeout(() => fn(...a), ms); };
}

export function fmtTime(sec: number): string {
  sec = Math.max(0, Math.floor(sec || 0));
  const hh = Math.floor(sec / 3600), mm = Math.floor((sec % 3600) / 60), ss = sec % 60;
  const p = (n: number) => (n < 10 ? '0' : '') + n;
  return hh ? `${hh}:${p(mm)}:${p(ss)}` : `${mm}:${p(ss)}`;
}
