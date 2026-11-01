'use strict';

const TOKEN = document.body.dataset.token;
const $ = (id) => document.getElementById(id);
const audio = $('audio');
const SPEEDS = [0.75, 1, 1.25, 1.5, 2];
// Same shape as the markers written by the transcription pipeline:
// [疑似：詞，mm:ss.mmm]. The word itself may contain commas, so the time anchors the match.
const SUSPECT_RE = /\[疑似[：:]([\s\S]*?)[，,](\d+(?::\d{1,3}){1,2}\.\d{3})\]/g;

let S = null;            // last state from the server
let base = [];           // saved body per segment id
const edits = new Map(); // segment id -> unsaved text
let review = { segments: {}, suspects: {} };
let rows = [];           // row element per segment id
let cur = 0;             // current segment id
let pin = null;          // {id, ms}: keep `cur` on id until playback reaches ms
let editingId = null;
let loadedAudio = '';
let reviewTimer = null;
let speed = 1;

// ---------- helpers ----------
async function api(method, path, body) {
  const res = await fetch(path, {
    method,
    headers: { 'X-Token': TOKEN, 'Content-Type': 'application/json' },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  const data = await res.json().catch(() => ({}));
  if (!res.ok) {
    const err = new Error(data.error || res.statusText);
    err.code = data.code;
    throw err;
  }
  return data;
}

function store(key, val) {
  try { if (val === null) localStorage.removeItem(key); else localStorage.setItem(key, val); } catch (e) { /* ignore */ }
}
function load(key) {
  try { return localStorage.getItem(key); } catch (e) { return null; }
}

let toastTimer = null;
function toast(msg) {
  const t = $('toast');
  t.textContent = msg;
  t.classList.add('show');
  clearTimeout(toastTimer);
  toastTimer = setTimeout(() => t.classList.remove('show'), 2200);
}

const pad = (n, w = 2) => String(n).padStart(w, '0');
function fmtClock(sec, frac) {
  sec = Math.max(0, sec || 0);
  const m = Math.floor(sec / 60), s = sec - m * 60;
  return pad(m) + ':' + (frac ? pad(s.toFixed(1), 4) : pad(Math.floor(s)));
}
function parseMs(str) {
  const [hms, ms] = str.split('.');
  return hms.split(':').reduce((a, p) => a * 60 + Number(p), 0) * 1000 + Number(ms);
}

const textOf = (id) => (edits.has(id) ? edits.get(id) : base[id]);
const leadSec = () => Number($('lead').value) || 0;

function suspectsOf(text) {
  const out = [];
  for (const m of text.matchAll(SUSPECT_RE)) {
    out.push({ raw: m[0], text: m[1], ms: parseMs(m[2]), index: m.index });
  }
  return out;
}
function allSuspects() {
  const out = [];
  if (!S) return out;
  for (const seg of S.segments) {
    for (const s of suspectsOf(textOf(seg.id))) out.push({ ...s, segId: seg.id });
  }
  return out;
}

// ---------- state / rendering ----------
function adopt(st) {
  S = st;
  base = st.segments.map((s) => s.body);
  edits.clear();
  review = st.review || {};
  review.segments = review.segments || {};
  review.suspects = review.suspects || {};
  cur = 0; pin = null; editingId = null;
  $('fileName').textContent = st.open ? st.mdName : '尚未開啟檔案';
  buildList();
  setAudio();
  restoreDraft();
  refreshAll();
}

function buildList() {
  const list = $('list');
  list.textContent = '';
  rows = [];
  if (!S.open) {
    list.innerHTML = '<p class="empty">按左上角「開啟」選擇逐字稿 (.md)，或把檔案拖進來。</p>';
    return;
  }
  if (S.segments.length === 0) {
    list.innerHTML = '<p class="empty">這個檔案裡找不到以 [mm:ss.mmm] 開頭的段落（內容原樣保留，不會被改動）。</p>';
    return;
  }
  for (const seg of S.segments) {
    const row = document.createElement('div');
    row.className = 'seg';
    row.id = 'seg-' + seg.id;

    const head = document.createElement('div');
    head.className = 'segHead';
    const t = document.createElement('button');
    t.className = 't';
    t.textContent = seg.time;
    t.title = '跳到本段並播放';
    t.addEventListener('click', () => gotoSeg(seg.id, { play: true }));
    const lab = document.createElement('label');
    const cb = document.createElement('input');
    cb.type = 'checkbox';
    cb.addEventListener('change', () => {
      setReview('segments', seg.startMs, cb.checked);
    });
    lab.append(cb, ' 已校對');
    const rev = document.createElement('button');
    rev.className = 'revert';
    rev.textContent = '還原';
    rev.title = '放棄本段未存的修改';
    rev.hidden = true;
    rev.addEventListener('click', () => {
      if (editingId === seg.id) endEdit();
      edits.delete(seg.id);
      saveDraft();
      renderBody(seg.id);
      refreshAll();
    });
    head.append(t, lab, rev);

    const body = document.createElement('div');
    body.className = 'body';
    body.addEventListener('click', () => {
      if (!window.getSelection().isCollapsed) return; // user is selecting text to copy
      cur = seg.id;
      startEdit(seg.id);
    });

    row.append(head, body);
    list.append(row);
    rows[seg.id] = row;
    renderBody(seg.id);
  }
}

function renderBody(id) {
  const el = rows[id].querySelector('.body');
  const text = textOf(id);
  el.textContent = '';
  let pos = 0;
  for (const s of suspectsOf(text)) {
    if (s.index > pos) el.append(text.slice(pos, s.index));
    const mark = document.createElement('mark');
    mark.className = 'sus' + (review.suspects[s.ms] ? ' done' : '');
    mark.textContent = s.raw;
    mark.title = '點一下回聽 ' + fmtClock(s.ms / 1000, true);
    mark.addEventListener('click', (e) => {
      e.stopPropagation();
      jumpToSuspect(s.ms, id);
    });
    el.append(mark);
    pos = s.index + s.raw.length;
  }
  if (pos < text.length) el.append(text.slice(pos));
}

function refreshAll() {
  if (!S || !S.open) {
    $('dirty').textContent = '';
    $('progress').textContent = '';
    $('suspects').textContent = '';
    document.title = 'sermon-proof';
    return;
  }
  for (const seg of S.segments) {
    const row = rows[seg.id];
    if (!row) continue;
    row.classList.toggle('edited', edits.has(seg.id));
    row.classList.toggle('active', seg.id === cur);
    const done = !!review.segments[seg.startMs];
    row.classList.toggle('done', done);
    row.querySelector('input[type=checkbox]').checked = done;
    row.querySelector('.revert').hidden = !edits.has(seg.id);
  }
  $('dirty').textContent = edits.size ? `● 未存 ${edits.size} 段` : (S.hasOrig ? '已有 .orig.md 備份' : '');
  document.title = (edits.size ? '● ' : '') + S.mdName;
  renderSuspects();
}

function renderSuspects() {
  const list = $('suspects');
  const all = allSuspects();
  const doneN = all.filter((s) => review.suspects[s.ms]).length;
  const segDone = S.segments.filter((s) => review.segments[s.startMs]).length;
  $('progress').textContent = `已校對 ${segDone}/${S.segments.length} 段 · 存疑已確認 ${doneN}/${all.length}`;
  const onlyOpen = $('onlyOpen').checked;
  list.textContent = '';
  let shown = 0;
  for (const s of all) {
    const done = !!review.suspects[s.ms];
    if (onlyOpen && done) continue;
    shown++;
    const li = document.createElement('li');
    li.className = done ? 'done' : '';
    const cb = document.createElement('input');
    cb.type = 'checkbox';
    cb.checked = done;
    cb.title = '已確認（工具不會自動勾選）';
    cb.addEventListener('click', (e) => e.stopPropagation());
    cb.addEventListener('change', () => {
      setReview('suspects', s.ms, cb.checked);
    });
    const st = document.createElement('div');
    st.className = 'st';
    st.append(s.text);
    const ts = document.createElement('div');
    ts.className = 'ts mono';
    ts.textContent = fmtClock(s.ms / 1000, true);
    st.prepend(ts);
    const un = document.createElement('button');
    un.className = 'un';
    un.textContent = '解除';
    un.title = '把 [疑似：詞，時間] 換成「詞」（需再存檔）';
    un.addEventListener('click', (e) => {
      e.stopPropagation();
      removeSuspect(s);
    });
    li.append(cb, st, un);
    const opts = optionsOf(s.text);
    if (opts.length > 1) {
      // Several candidate wordings joined by ／: let the user keep just one.
      const box = document.createElement('div');
      box.className = 'opts';
      for (const o of opts) {
        const b = document.createElement('button');
        b.className = 'opt';
        b.textContent = o;
        b.title = '只用這個：把 [疑似：…] 換成「' + o + '」（需再存檔）';
        b.addEventListener('click', (e) => {
          e.stopPropagation();
          removeSuspect(s, o);
        });
        box.append(b);
      }
      li.append(box);
      un.title = '兩個以上的候選都保留：換成「' + s.text + '」（需再存檔）';
    }
    li.addEventListener('click', () => jumpToSuspect(s.ms, s.segId));
    list.append(li);
  }
  if (!shown) {
    const li = document.createElement('li');
    li.className = 'none';
    li.textContent = all.length ? '全部已確認' : '沒有存疑標記';
    list.append(li);
  }
}

function setReview(kind, key, on) {
  if (on) review[kind][key] = true; else delete review[kind][key];
  clearTimeout(reviewTimer);
  reviewTimer = setTimeout(() => {
    api('PUT', '/api/review', review).catch((e) => toast('勾選狀態未能儲存：' + e.message));
  }, 400);
  for (const seg of S.segments) renderBody(seg.id); // suspect colours
  refreshAll();
}

// Candidates inside a marker are separated by a full-width slash: 決志／覺知.
function optionsOf(word) {
  return word.split('／').filter((o) => o !== '');
}

function removeSuspect(s, replacement = s.text) {
  const text = textOf(s.segId);
  const i = text.indexOf(s.raw);
  if (i < 0) return;
  setText(s.segId, text.slice(0, i) + replacement + text.slice(i + s.raw.length));
  if (editingId === s.segId) rows[s.segId].querySelector('textarea').value = textOf(s.segId);
  renderBody(s.segId);
  refreshAll();
}

// ---------- editing ----------
function setText(id, text) {
  if (text === base[id]) edits.delete(id); else edits.set(id, text);
  saveDraft();
}

function autosize(ta) {
  ta.style.height = 'auto';
  ta.style.height = ta.scrollHeight + 2 + 'px';
}

function startEdit(id) {
  if (editingId === id) return;
  if (editingId !== null) endEdit();
  const row = rows[id];
  const view = row.querySelector('.body');
  const ta = document.createElement('textarea');
  ta.className = 'editor';
  ta.rows = 1;
  ta.value = textOf(id);
  ta.spellcheck = false;
  view.hidden = true;
  view.after(ta);
  editingId = id;
  ta.addEventListener('input', () => {
    setText(id, ta.value);
    autosize(ta);
    row.classList.toggle('edited', edits.has(id));
    row.querySelector('.revert').hidden = !edits.has(id);
    $('dirty').textContent = edits.size ? `● 未存 ${edits.size} 段` : '';
    renderSuspects();
  });
  ta.addEventListener('keydown', (e) => onEditKey(e, id));
  ta.addEventListener('blur', () => { if (editingId === id) endEdit(); });
  autosize(ta);
  ta.focus();
  ta.setSelectionRange(ta.value.length, ta.value.length);
}

function endEdit() {
  if (editingId === null) return;
  const id = editingId;
  editingId = null;
  const row = rows[id];
  row.querySelector('textarea')?.remove();
  row.querySelector('.body').hidden = false;
  renderBody(id);
  refreshAll();
}

function moveEdit(delta) {
  const id = Math.min(S.segments.length - 1, Math.max(0, editingId + delta));
  endEdit();
  gotoSeg(id, {});
  startEdit(id);
}

function onEditKey(e, id) {
  if (e.isComposing || e.keyCode === 229) return; // IME in progress
  const meta = e.metaKey || e.ctrlKey;
  if (e.key === 'Enter') {
    e.preventDefault(); // segments stay single-paragraph; ⌘Enter finishes
    if (meta) endEdit();
  } else if (e.key === 'Escape') {
    e.preventDefault();
    togglePlay();
  } else if (meta && e.key === 'ArrowUp') {
    e.preventDefault(); moveEdit(-1);
  } else if (meta && e.key === 'ArrowDown') {
    e.preventDefault(); moveEdit(1);
  } else if (meta && e.key === '[') {
    e.preventDefault(); seekBy(-3);
  } else if (meta && e.key === ']') {
    e.preventDefault(); seekBy(3);
  }
}

// ---------- drafts (survive a crashed tab; not the saved file) ----------
const draftKey = () => 'draft:' + S.mdPath;
function saveDraft() {
  if (!S || !S.open) return;
  store(draftKey(), edits.size ? JSON.stringify({ hash: S.hash, edits: [...edits] }) : null);
}
function restoreDraft() {
  if (!S.open) return;
  const raw = load(draftKey());
  if (!raw) return;
  try {
    const d = JSON.parse(raw);
    if (d.hash === S.hash && d.edits.length && confirm(`發現 ${d.edits.length} 段未存的草稿，要還原嗎？`)) {
      for (const [id, text] of d.edits) if (id < base.length) setText(id, text);
      for (const seg of S.segments) renderBody(seg.id);
      return;
    }
  } catch (e) { /* fall through */ }
  store(draftKey(), null);
}

// ---------- open / save ----------
async function guardDirty() {
  return edits.size === 0 || confirm('有未存的修改，確定要放棄嗎？');
}

async function pickMd() {
  if (!(await guardDirty())) return;
  try {
    const st = await api('POST', '/api/pick-md', {});
    if (st.cancelled) return;
    adopt(st);
    if (!st.hasAudio) toast('找不到對應的 mp3，請按「音檔」');
  } catch (e) { alert(e.message); }
}

async function pickAudio() {
  if (!S || !S.open) { toast('請先開啟逐字稿'); return; }
  try {
    const st = await api('POST', '/api/pick-audio', {});
    if (st.cancelled) return;
    S = st; // keep unsaved edits
    setAudio();
  } catch (e) { alert(e.message); }
}

// ---------- drag & drop ----------
// A page never sees a dropped file's path, so send name/size/mtime (plus any
// file:// URIs the browser offers) and let the server work out which file it is.
const AUDIO_EXT = /\.(mp3|m4a|wav)$/i;
let dragDepth = 0;

function hasFiles(e) {
  return e.dataTransfer && [...e.dataTransfer.types].includes('Files');
}

async function openDropped(files, uris) {
  files = files.filter((f) => f.name);
  if (!files.length) return;
  if (files.some((f) => /\.md$/i.test(f.name)) && !(await guardDirty())) return;
  try {
    const st = await api('POST', '/api/open-dropped', {
      files: files.map((f) => ({ name: f.name, size: f.size, mtimeMs: f.lastModified })),
      uris,
    });
    if (files.some((f) => /\.md$/i.test(f.name))) {
      adopt(st);
      if (!st.hasAudio) toast('找不到對應的 mp3，請按「音檔」或一起拖入');
    } else {
      S = st; // audio only: keep unsaved edits
      setAudio();
    }
  } catch (e) { alert(e.message); }
}

document.addEventListener('dragenter', (e) => {
  if (!hasFiles(e)) return;
  e.preventDefault();
  dragDepth++;
  document.body.classList.add('dropping');
});
document.addEventListener('dragover', (e) => {
  if (!hasFiles(e)) return;
  e.preventDefault(); // without this the browser would navigate to / play the file
  e.dataTransfer.dropEffect = 'copy';
});
document.addEventListener('dragleave', (e) => {
  if (!hasFiles(e)) return;
  if (--dragDepth <= 0) { dragDepth = 0; document.body.classList.remove('dropping'); }
});
document.addEventListener('drop', (e) => {
  if (!hasFiles(e)) return;
  e.preventDefault();
  dragDepth = 0;
  document.body.classList.remove('dropping');
  const uris = (e.dataTransfer.getData('text/uri-list') || '').split(/\r?\n/).filter((l) => l && !l.startsWith('#'));
  openDropped([...e.dataTransfer.files], uris);
});

async function save() {
  if (!S || !S.open) return;
  if (!edits.size) { toast('沒有變更'); return; }
  try {
    const st = await api('POST', '/api/save', {
      baseHash: S.hash,
      edits: [...edits].map(([id, text]) => ({ id, text })),
    });
    S.hash = st.hash;
    S.hasOrig = st.hasOrig;
    base = st.segments.map((s) => s.body);
    edits.clear();
    saveDraft();
    refreshAll();
    toast('已存檔');
  } catch (e) {
    if (e.code === 'conflict') {
      if (confirm('磁碟上的檔案在載入後被外部改動過，無法安全存檔。\n要重新載入嗎？（會放棄畫面上未存的修改）')) {
        edits.clear();
        saveDraft();
        adopt(await api('POST', '/api/reload', {}));
      }
    } else {
      alert('存檔失敗：' + e.message);
    }
  }
}

// ---------- audio ----------
function setAudio() {
  const has = S && S.hasAudio;
  $('noAudio').hidden = !(S && S.open && !has);
  for (const id of ['btnPrev', 'btnBack', 'btnPlay', 'btnFwd', 'btnNext', 'btnReplay', 'seek']) $(id).disabled = !has;
  if (!has) {
    if (loadedAudio) { audio.removeAttribute('src'); audio.load(); }
    loadedAudio = '';
    return;
  }
  if (loadedAudio !== S.audioPath) {
    loadedAudio = S.audioPath;
    audio.src = '/api/audio?t=' + TOKEN + '&p=' + encodeURIComponent(S.audioPath);
    audio.load();
  }
  applySpeed();
}

function applySpeed() {
  audio.playbackRate = speed;
  audio.preservesPitch = true;
  audio.webkitPreservesPitch = true;
  for (const b of $('speeds').children) b.classList.toggle('on', Number(b.dataset.v) === speed);
}

function togglePlay() {
  if (!S || !S.hasAudio) return;
  if (audio.paused) audio.play().catch(() => {}); else audio.pause();
}

function segAt(ms) {
  const segs = S.segments;
  let lo = 0, hi = segs.length - 1, ans = 0;
  while (lo <= hi) {
    const mid = (lo + hi) >> 1;
    if (segs[mid].startMs <= ms) { ans = mid; lo = mid + 1; } else hi = mid - 1;
  }
  return ans;
}

function seekToMs(ms, id) {
  const t = Math.max(0, ms / 1000 - leadSec());
  pin = { id, ms };
  audio.currentTime = t;
  setCur(id, true);
  updateUI();
}

function gotoSeg(id, { play = false }) {
  if (!S || !S.segments.length) return;
  id = Math.min(S.segments.length - 1, Math.max(0, id));
  if (S.hasAudio) {
    seekToMs(S.segments[id].startMs, id);
    if (play) audio.play().catch(() => {});
  } else {
    setCur(id, true);
  }
}

function jumpToSuspect(ms, segId) {
  if (!S.hasAudio) { toast('沒有音檔'); return; }
  seekToMs(ms, segId);
  audio.play().catch(() => {});
  flash(segId);
}

function flash(id) {
  const row = rows[id];
  if (!row) return;
  row.classList.add('flash');
  setTimeout(() => row.classList.remove('flash'), 900);
}

function seekBy(sec) {
  if (!S || !S.hasAudio) return;
  pin = null;
  audio.currentTime = Math.min(audio.duration || Infinity, Math.max(0, audio.currentTime + sec));
  updateUI();
}

function setCur(id, force) {
  const changed = id !== cur;
  cur = id;
  if (changed || force) {
    rows.forEach((r, i) => r && r.classList.toggle('active', i === id));
    const row = rows[id];
    if (row && (force || editingId === null)) {
      const r = row.getBoundingClientRect();
      const barH = $('bar').offsetHeight;
      if (r.top < barH + 8 || r.bottom > window.innerHeight - 8) {
        row.scrollIntoView({ block: 'center', behavior: 'smooth' });
      }
    }
  }
}

function updateUI() {
  if (!S || !S.hasAudio) return;
  const t = audio.currentTime;
  const ms = t * 1000;
  if (pin && ms >= pin.ms) pin = null;
  if (S.segments.length) setCur(pin ? pin.id : segAt(ms), false);
  $('cur').textContent = fmtClock(t, true);
  if (isFinite(audio.duration)) {
    $('dur').textContent = fmtClock(audio.duration, false);
    $('seek').max = audio.duration;
  }
  if (document.activeElement !== $('seek')) $('seek').value = t;
}

let raf = 0;
function loop() {
  updateUI();
  if (!audio.paused) raf = requestAnimationFrame(loop);
}

audio.addEventListener('play', () => {
  $('btnPlay').textContent = '⏸ 暫停';
  cancelAnimationFrame(raf);
  raf = requestAnimationFrame(loop);
});
audio.addEventListener('pause', () => {
  $('btnPlay').textContent = '▶ 播放';
  savePos();
});
audio.addEventListener('ended', () => { $('btnPlay').textContent = '▶ 播放'; });
audio.addEventListener('timeupdate', updateUI);
audio.addEventListener('seeked', updateUI);
audio.addEventListener('loadedmetadata', () => {
  applySpeed();
  const p = Number(load('pos:' + S.mdPath));
  if (p > 0 && p < audio.duration) {
    audio.currentTime = p;
    if (S.segments.length) setCur(segAt(p * 1000), true);
  }
  updateUI();
});
audio.addEventListener('error', () => toast('音檔載入失敗'));

function savePos() {
  if (S && S.open && S.hasAudio) store('pos:' + S.mdPath, String(audio.currentTime));
}

// ---------- suspects navigation ----------
function nextSuspect() {
  const open = allSuspects().filter((s) => !review.suspects[s.ms]);
  if (!open.length) { toast('沒有未確認的存疑處'); return; }
  const now = audio.currentTime * 1000;
  const next = open.find((s) => s.ms > now + 500 + leadSec() * 1000) || open[0];
  jumpToSuspect(next.ms, next.segId);
}

// ---------- wiring ----------
$('btnOpen').addEventListener('click', pickMd);
$('btnAudio').addEventListener('click', pickAudio);
$('btnSave').addEventListener('click', save);
$('btnPlay').addEventListener('click', togglePlay);
$('btnBack').addEventListener('click', () => seekBy(-3));
$('btnFwd').addEventListener('click', () => seekBy(3));
$('btnPrev').addEventListener('click', () => gotoSeg(cur - 1, {}));
$('btnNext').addEventListener('click', () => gotoSeg(cur + 1, {}));
$('btnReplay').addEventListener('click', () => gotoSeg(cur, { play: true }));
$('onlyOpen').addEventListener('change', () => S && S.open && renderSuspects());
$('seek').addEventListener('input', () => { pin = null; audio.currentTime = Number($('seek').value); updateUI(); });
$('lead').addEventListener('change', () => store('lead', $('lead').value));

for (const v of SPEEDS) {
  const b = document.createElement('button');
  b.textContent = v + '×';
  b.dataset.v = v;
  b.addEventListener('click', () => { speed = v; store('speed', String(v)); applySpeed(); });
  $('speeds').append(b);
}

// Buttons keep focus after a click, which would swallow the space/arrow shortcuts.
document.addEventListener('click', (e) => {
  const el = e.target.closest('button, input[type=checkbox], input[type=range], select');
  if (el) el.blur();
});

document.addEventListener('keydown', (e) => {
  if (e.isComposing || e.keyCode === 229) return;
  if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 's') {
    e.preventDefault();
    save();
    return;
  }
  const tag = e.target.tagName;
  if (tag === 'TEXTAREA' || tag === 'INPUT' || tag === 'SELECT' || tag === 'BUTTON') return;
  if (e.metaKey || e.ctrlKey || e.altKey || !S || !S.open) return;
  const k = e.key;
  let used = true;
  if (k === ' ') togglePlay();
  else if (k === 'ArrowLeft') seekBy(-3);
  else if (k === 'ArrowRight') seekBy(3);
  else if (k === 'ArrowUp') gotoSeg(cur - 1, {});
  else if (k === 'ArrowDown') gotoSeg(cur + 1, {});
  else if (k === 'r' || k === 'R') gotoSeg(cur, { play: true });
  else if (k === '[') { speed = SPEEDS[Math.max(0, SPEEDS.indexOf(speed) - 1)]; store('speed', String(speed)); applySpeed(); }
  else if (k === ']') { speed = SPEEDS[Math.min(SPEEDS.length - 1, SPEEDS.indexOf(speed) + 1)]; store('speed', String(speed)); applySpeed(); }
  else if (k === 'Enter') { if (S.segments.length) startEdit(cur); }
  else if (k === 'n' || k === 'N') nextSuspect();
  else if (k === 'c' || k === 'C') {
    const seg = S.segments[cur];
    if (seg) setReview('segments', seg.startMs, !review.segments[seg.startMs]);
  } else used = false;
  if (used) e.preventDefault();
});

window.addEventListener('beforeunload', (e) => {
  savePos();
  if (edits.size) { e.preventDefault(); e.returnValue = ''; }
});

(async function init() {
  const sp = Number(load('speed'));
  if (SPEEDS.includes(sp)) speed = sp;
  const ld = load('lead');
  if (ld !== null && [...$('lead').options].some((o) => o.value === ld)) $('lead').value = ld;
  try {
    adopt(await api('GET', '/api/state'));
  } catch (e) {
    alert('無法連到後端：' + e.message);
  }
})();
