// Playback capabilities reported to the server (POST /api/items/{id}/playback "caps").
// Probing (MediaSource.isTypeSupported / canPlayType) is combined with a per-platform
// table, because TV engines often answer "" for formats their native pipeline plays
// fine from a plain <video src> (MKV, AC-3/E-AC-3, HEVC), and direct play should win
// as often as it safely can.
import type { Platform } from '../platform';

export interface Caps { video: string[]; audio: string[]; containers: string[]; hdr: boolean }

/**
 * Known-good formats per platform when the file is given to <video src> directly.
 * Sources: Samsung Tizen media specs (2018+ TVs: Tizen 4.0), LG webOS TV media formats
 * (webOS 4.x), Apple HLS/AVFoundation, Android media formats (MediaCodec via Chromium).
 * Video and audio entries here are added on top of what probing finds; the table is
 * also the test fixture in test/caps.test.mjs.
 */
export const PLATFORM_CAPS: Record<Platform, Caps> = {
  tizen: { video: ['h264', 'hevc', 'mpeg4', 'vp9'], audio: ['aac', 'mp3', 'ac3', 'eac3', 'flac', 'opus'], containers: ['mp4', 'mkv', 'webm', 'mov', 'ts', 'm2ts', 'avi'], hdr: false },
  webos: { video: ['h264', 'hevc', 'vp9'], audio: ['aac', 'mp3', 'ac3', 'eac3', 'flac'], containers: ['mp4', 'mkv', 'webm', 'mov', 'ts', 'm2ts'], hdr: false },
  androidtv: { video: ['h264'], audio: ['aac', 'mp3', 'opus', 'flac'], containers: ['mp4', 'webm'], hdr: false },
  android: { video: ['h264'], audio: ['aac', 'mp3', 'opus', 'flac'], containers: ['mp4', 'webm'], hdr: false },
  ios: { video: ['h264', 'hevc'], audio: ['aac', 'mp3', 'ac3', 'eac3', 'flac'], containers: ['mp4', 'mov'], hdr: false },
  web: { video: [], audio: [], containers: [], hdr: false },
};

/** HDR10 is only claimed where the platform can say the panel supports it. */
function platformHDR(p: Platform): boolean {
  const w = window as any;
  try {
    if (p === 'tizen' && w.webapis && w.webapis.avinfo && w.webapis.avinfo.isHdrTvSupport) return !!w.webapis.avinfo.isHdrTvSupport();
  } catch { /* old firmware */ }
  try {
    return !!((window as any).matchMedia && matchMedia('(dynamic-range: high)').matches);
  } catch { return false; }
}

export function probeCaps(): Caps {
  const v = document.createElement('video');
  const ms = (window as any).MediaSource || (window as any).ManagedMediaSource;
  const can = (t: string) => {
    try { if (ms && ms.isTypeSupported && ms.isTypeSupported(t)) return true; } catch { /* */ }
    return v.canPlayType(t) === 'probably';
  };
  const c: Caps = { video: [], audio: [], containers: [], hdr: false };
  if (can('video/mp4; codecs="avc1.640028"')) c.video.push('h264');
  if (can('video/mp4; codecs="hvc1.1.6.L120.90"') || can('video/mp4; codecs="hev1.1.6.L120.90"')) c.video.push('hevc');
  if (can('video/webm; codecs="vp9"') || can('video/mp4; codecs="vp09.00.10.08"')) c.video.push('vp9');
  if (can('video/mp4; codecs="av01.0.05M.08"')) c.video.push('av1');
  if (can('audio/mp4; codecs="mp4a.40.2"')) c.audio.push('aac');
  if (can('audio/mpeg')) c.audio.push('mp3');
  if (can('audio/webm; codecs="opus"') || can('audio/mp4; codecs="opus"')) c.audio.push('opus');
  if (can('audio/flac') || can('audio/mp4; codecs="flac"')) c.audio.push('flac');
  if (can('audio/mp4; codecs="ac-3"')) c.audio.push('ac3');
  if (can('audio/mp4; codecs="ec-3"')) c.audio.push('eac3');
  if (v.canPlayType('video/mp4')) c.containers.push('mp4');
  if (v.canPlayType('video/webm')) c.containers.push('webm');
  if (v.canPlayType('video/quicktime')) c.containers.push('mov');
  if (v.canPlayType('video/x-matroska') === 'probably') c.containers.push('mkv');
  return c;
}

/** Union of probe and platform table; HDR from the platform. */
export function mergeCaps(probed: Caps, p: Platform, hdr: boolean): Caps {
  const t = PLATFORM_CAPS[p];
  const uni = (a: string[], b: string[]) => a.concat(b.filter(x => a.indexOf(x) < 0));
  return { video: uni(probed.video, t.video), audio: uni(probed.audio, t.audio), containers: uni(probed.containers, t.containers), hdr: hdr && uni(probed.video, t.video).indexOf('hevc') >= 0 };
}

export function detectCaps(p: Platform): Caps {
  return mergeCaps(probeCaps(), p, platformHDR(p));
}

/** Native HLS in <video> (Tizen, webOS, iOS/Safari); elsewhere hls.js over MSE. */
export function nativeHLS(p: Platform): boolean {
  if (p === 'tizen' || p === 'webos' || p === 'ios') return true;
  const v = document.createElement('video');
  return !(window as any).MediaSource && !!v.canPlayType('application/vnd.apple.mpegurl');
}
