'use strict';
// Pure display / lookup helpers for WebVTT transcripts. No DOM, no fetch, no
// /api/* — so the same file can drive a static page that only has an <audio>
// element and a .vtt (cues from a TextTrack work too: they have startMs-like
// startTime, map them to {startMs, endMs} first). Loaded as a classic script
// (global `VTTView`) or via require() when a JS runtime offers it.
(function (root) {
  // Markers written by the transcription pipeline: [疑似：詞，mm:ss.mmm].
  // The word itself may contain commas, so the trailing time anchors the match.
  const SUSPECT_SRC = '\\[疑似[：:]([\\s\\S]*?)[，,](\\d+(?::\\d{1,3}){1,2}\\.\\d{3})\\]';

  const pad = (n, w = 2) => String(n).padStart(w, '0');

  // "mm:ss.mmm" or "h:mm:ss.mmm" -> milliseconds.
  function parseTimeMs(str) {
    const [hms, ms] = str.split('.');
    return hms.split(':').reduce((a, p) => a * 60 + Number(p), 0) * 1000 + Number(ms);
  }

  // milliseconds -> "mm:ss.mmm" ("hh:mm:ss.mmm" from one hour), the WebVTT form.
  function formatTime(ms) {
    ms = Math.max(0, Math.round(ms || 0));
    const h = Math.floor(ms / 3600000), m = Math.floor(ms / 60000) % 60;
    const s = Math.floor(ms / 1000) % 60, f = ms % 1000;
    return (h ? pad(h) + ':' : '') + pad(m) + ':' + pad(s) + '.' + pad(f, 3);
  }

  // Index of the last cue whose startMs <= ms, or -1 before the first cue.
  // `cues` must be ordered by startMs; only startMs is used, so a cue's own
  // endMs (which may overlap or leave gaps in a foreign file) never matters.
  function cueIndexAt(cues, ms) {
    let lo = 0, hi = cues.length - 1, ans = -1;
    while (lo <= hi) {
      const mid = (lo + hi) >> 1;
      if (cues[mid].startMs <= ms) { ans = mid; lo = mid + 1; } else hi = mid - 1;
    }
    return ans;
  }

  // meta is the NOTE block as [{Key, Value}] (null when there is none).
  function metaGet(meta, key) {
    const kv = (meta || []).find((x) => x.Key === key);
    return kv ? kv.Value : '';
  }

  // "雅各書 (N59) 5:10–11 · 康來昌" from the book / passage / speaker keys; '' if none present.
  function metaSummary(meta) {
    const place = [metaGet(meta, 'book'), metaGet(meta, 'passage')].filter(Boolean).join(' ');
    return [place, metaGet(meta, 'speaker')].filter(Boolean).join(' · ');
  }

  // Every NOTE line as "key: value", for a tooltip.
  function metaText(meta) {
    return (meta || []).map((x) => x.Key + ': ' + x.Value).join('\n');
  }

  // [{raw, text, ms, index}] for each suspect marker in text.
  function findSuspects(text) {
    const out = [];
    for (const m of text.matchAll(new RegExp(SUSPECT_SRC, 'g'))) {
      out.push({ raw: m[0], text: m[1], ms: parseTimeMs(m[2]), index: m.index });
    }
    return out;
  }

  // text cut into [{type:'text', text} | {type:'suspect', text, raw, ms}] for highlighting.
  function splitSuspects(text) {
    const parts = [];
    let pos = 0;
    for (const s of findSuspects(text)) {
      if (s.index > pos) parts.push({ type: 'text', text: text.slice(pos, s.index) });
      parts.push({ type: 'suspect', text: s.text, raw: s.raw, ms: s.ms });
      pos = s.index + s.raw.length;
    }
    if (pos < text.length) parts.push({ type: 'text', text: text.slice(pos) });
    return parts;
  }

  const api = { parseTimeMs, formatTime, cueIndexAt, metaGet, metaSummary, metaText, findSuspects, splitSuspects };
  if (typeof module !== 'undefined' && module.exports) module.exports = api;
  else root.VTTView = api;
})(typeof self !== 'undefined' ? self : this);
