// WebVTT from the server, rendered by the app itself: <track> is unreliable on
// 2018 TV engines (cross-origin loading, styling, cue timing with HLS offsets).
export interface Cue { start: number; end: number; text: string }

function ts(s: string): number {
  const p = s.trim().split(':');
  let sec = 0;
  for (const x of p) sec = sec * 60 + parseFloat(x.replace(',', '.'));
  return sec;
}

export function parseVTT(src: string): Cue[] {
  const out: Cue[] = [];
  const blocks = src.replace(/\r/g, '').split(/\n\n+/);
  for (const b of blocks) {
    const lines = b.split('\n');
    let i = -1;
    for (let k = 0; k < lines.length; k++) if (lines[k].indexOf('-->') >= 0) { i = k; break; }
    if (i < 0) continue;
    const parts = lines[i].split('-->');
    const text = lines.slice(i + 1).join('\n').replace(/<[^>]+>/g, '').replace(/&amp;/g, '&').replace(/&lt;/g, '<').replace(/&gt;/g, '>').replace(/&nbsp;/g, ' ').trim();
    if (text) out.push({ start: ts(parts[0]), end: ts(parts[1].trim().split(/\s+/)[0]), text });
  }
  out.sort((a, b) => a.start - b.start);
  return out;
}

/** Binary-searches the cues active at t (seconds on the item timeline). */
export function activeCues(cues: Cue[], t: number): Cue[] {
  let lo = 0, hi = cues.length;
  while (lo < hi) { const m = (lo + hi) >> 1; if (cues[m].start <= t) lo = m + 1; else hi = m; }
  const out: Cue[] = [];
  for (let i = lo - 1; i >= 0 && i >= lo - 8; i--) if (cues[i].end > t) out.unshift(cues[i]);
  return out;
}
