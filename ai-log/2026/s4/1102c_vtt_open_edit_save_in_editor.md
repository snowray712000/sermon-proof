# Open, proofread and safely save WebVTT (.vtt) transcripts in sermon-proof

- Date: 2026-11-02
- Project: sermon-proof
- Type: feature
- Related logs: 1102b_webvtt_intermediate_format_and_converter.md, 1102a_unify_n59_file_naming.md, 1026b_initial_implementation.md
- Cross-project: data folder `../語音辨識/信望愛/講道/康來昌/N59_雅各書/` (read only here; two files copied into `testdata/`)
- Commit: uncommitted
- Version: 1.3.1 -> 1.4.0

## Goal / why
After 1102b the 34 N59 sermons exist as `X-ver1.vtt` (proofread) and `X-ver1.orig.vtt` (machine draft). The maintainer wants the same proofreading experience for `.vtt`, with byte-exact round-trip and the existing save safety, while `.md` keeps working. The `.vtt` will later also be used by a plain-web page (`<audio>` + TextTrack, no Go backend) and as AI context, so format logic must live in one place (`internal/transcript`) and new front-end logic must not depend on `/api/*`.

## Starting state
`transcript.ParseVTT`/`VTT` (vtt.go) is lossy (drops CRLF, BOM, STYLE/REGION/other NOTE, cue settings) and stays only for `tools/mdtovtt`. Session/server/dialogs/pairing were `.md`-only (`OpenMD`).

## What changed (behavior level)
- `sermon-proof X.vtt` / "開啟" dialog (md + vtt filter) / drag-drop open `.md` or `.vtt`. `.orig.md` and `.orig.vtt` are refused. A `.vtt` without a `WEBVTT` first line is refused ("not a WebVTT file").
- Same editing UX; title bar shows `book passage · speaker` from the NOTE block (tooltip = all NOTE lines), read only. The tool never edits NOTE / `status` / timings.
- Save: first save copies `X.orig.vtt` (never overwritten), temp file + rename, 409 when the disk hash differs — all by the existing `transcript.Save`.
- Version 1.4.0.

## Design and decisions
**One `Document`, two parsers** (chosen over a separate vtt type): `Document` gained `Format` ("md" | "vtt"; `Parse` sets "md"), `Meta []KV`; `Block` gained `EndMs`, `ReadOnly`. `ParseVTTDoc` (new `internal/transcript/vttdoc.go`) builds Raw blocks + one segment per cue; `Render`, `Apply`, `Clone`, session and API are shared. `Render` branches on `Format` for an *edited* cue only; untouched cues emit `Raw`, which is what guarantees byte-exact round-trip. `ParseByName(name, text)` picks the parser by extension.

**vtt parsing rules** (`ParseVTTDoc`): lines via `splitLines` (CRLF/LF per line; lone CR is not a terminator, so such text stays inside a line and still round-trips). First line, after an optional BOM, must be `WEBVTT` or start with `WEBVTT ` / `WEBVTT\t`, else `ErrNotVTT`. The header block (up to the first empty line) is Raw. Then blocks are runs of non-empty lines separated by empty lines (a line of only spaces is *not* blank, as in the spec). A block is:
- NOTE (`NOTE`, `NOTE `, `NOTE\t` first line): Raw; the first one before any cue supplies `Meta` (`key: value` lines cut at `": "`, order kept).
- A cue if line 0 matches the timing regexp `^((?:\d{2,}:)?\d{2}:\d{2}\.\d{3})[ \t]+-->[ \t]+(same)(?:[ \t].*)?$`, or line 0 has no `-->` and line 1 matches (line 0 = cue id, any text: not assumed numeric). Payload = following lines. If any payload line contains `-->` the whole block stays Raw (a player would split it).
- Anything else (STYLE, REGION, malformed timing, stray text): Raw. Raw runs merge, including blank lines.
A cue segment: `Prefix` = id line + timing line with their EOLs (cue settings stay inside it, untouched), `Time` = start text as in file, `StartMs`/`EndMs` from the timing line, `Body` = payload lines joined with their original inner EOLs, with `&amp; &lt; &gt;` undone, `EOL` = last payload line's terminator (the timing line's if the payload is empty), `Raw` = exact original bytes.

**Escaping**: display/edit text is unescaped; on save of an edited cue `vttEscape` (`& < >` -> entities) is applied. Cue payload from `tools/mdtovtt` always satisfies `escape(unescape(x)) == x`. A payload that does not (raw `<v Bob>`/`<b>` tags, a bare `&nbsp;`) would be corrupted by re-escaping, so such cues get `ReadOnly = true`: `Apply` rejects edits to them, the UI shows them but refuses to edit (toast). They still round-trip untouched.

**Edit rendering** (`renderVTTCue`): `Prefix` + escaped body + `EOL`. Body emptied -> only `Prefix` (no stray blank line). Payload-less cue whose timing line is the unterminated end of file -> a `d.EOL` is inserted before the new text. `Apply` (via `checkVTTEdit`) refuses a body with an empty line (`"a\n\nb"`, leading/trailing `\n`): in WebVTT an empty line ends the cue, so it would silently split it. Enter in the editor is still blocked, but pasting multi-line text can produce this; the save then fails with a message naming the cue time and all edits stay in the browser. Multi-line cues are preserved exactly (CRLF kept; `normalize` uses the dominant EOL for new line breaks).

**Names/paths** (`internal/transcript/pair.go`, `save.go`): `IsOrigName` knows `.orig.md` and `.orig.vtt`; `IsVTTName`, `IsTranscriptName` added; `PairAudio` strips `.md` or `.vtt` then the usual `.orig` / `-manual` / `-verN` suffix loop (`X-ver1.vtt` -> `X.mp3`). `OrigPath` unchanged: `X-ver1.vtt` -> `X-ver1.orig.vtt`. **`ReviewPath` would have collided** (`X-ver1.md` and `X-ver1.vtt` both -> `X-ver1.review.json`; keys are start ms so they would silently share ticks of two files that may diverge). For `.vtt` it is now `X-ver1.vtt.review.json`; the `.md` name is unchanged. Ticks made on the `.md` are therefore not carried over to the `.vtt` (the data folder has no `.review.json` files anyway).

**Session/API**: `Session.OpenMD` renamed `Open` (callers: main.go, server.go, tests); also parses before touching session state, so a failed open keeps the old file. `Session.Save` now parses the rendered output before writing to disk. `/api/*` routes and shapes unchanged; `State` adds `format` ("md"|"vtt") and `meta` (`[{Key,Value}]`, null for md); `Seg` adds `endMs` and `readOnly` (both `omitempty`; md has neither). `mdPath`/`mdName`/`/api/pick-md` names kept for compatibility (they now mean "transcript"). `filedlg` kind `"md"` now offers `md` + `vtt`. Drop: `dropKind(".vtt")` = transcript. main.go accepts a `.vtt` argument.

**Front end** (no rewrite of app.js): new `web/vttview.js` (classic script, global `VTTView`, also `module.exports`; embedded and routed `GET /vttview.js`; loaded before app.js) with pure functions `parseTimeMs`, `formatTime(ms)` (`mm:ss.mmm`/`hh:mm:ss.mmm`), `cueIndexAt(cues, ms)` (last cue with `startMs <= ms`, -1 if none; only start times are used so odd `endMs` in foreign files never matter), `metaGet`, `metaSummary`, `metaText`, `findSuspects` (the `[疑似：詞，mm:ss.mmm]` regex moved here, built fresh per call), `splitSuspects`. app.js uses them (`parseMs`, `suspectsOf`, `segAt`, title-bar `#meta`), plus `.orig.<format>` label, `TRANSCRIPT_EXT` for drops, and `readOnly` guards (click, Enter, ⌘↑/↓ via `startEdit`, 解除 via `removeSuspect`). No cue-count/id/single-line assumptions were added, so `<audio>` + `TextTrack` (`mode="hidden"`, `cuechange`) can drive it later.

## Key files / types / functions
`internal/transcript/vttdoc.go` (`ParseVTTDoc`, `addVTTBlock`, `checkVTTEdit`, `renderVTTCue`, `ParseByName`), `doc.go` (`Format`, `Meta`, `EndMs`, `ReadOnly`, `dominantEOL`), `pair.go`, `save.go`, `internal/server/session.go` (`Open`, `State`), `server.go`, `internal/filedlg/filedlg.go`, `main.go`, `web/vttview.js`, `web/app.js`, `web/index.html` (`#meta`), `web/embed.go`.

## Contracts and algorithms
- `ParseVTTDoc(s).Render() == s` for every `s` that starts with `WEBVTT` (tested: edge cases, 4000 random mixes of CRLF/BOM/NOTE/STYLE/REGION/tags/`-->`, both testdata files, all 34 real sermons).
- Editing cue *k* changes only cue *k*'s payload bytes; `Apply` ignores a text equal to the current body.
- `.md` behavior unchanged (`TestRoundTrip*`, `TestSessionSaveFlow` untouched except the rename).

## Gotchas
- `.orig.vtt` is refused on open, but the completion check in the task prompt says to open `N59_1_12_1_13-ver1.orig.vtt` (the one with `[疑似`). Resolution: copy it under another name (I used `N59_1_12_1_13-ver1.vtt` in the scratchpad); the first save then created a pre-save backup `X-ver1.orig.vtt` identical to the copy. Left to the maintainer if opening `.orig.vtt` should be allowed after all.
- In the real data folder `X-ver1.orig.vtt` already exists as the *machine draft*, so the first save of `X-ver1.vtt` keeps it (O_EXCL) and makes no pre-save backup — same as `.md` today. The draft is the intended `.orig`.
- A `.vtt` timing line with a 1-digit hour/minute or `,` decimal separator is not recognised: that block stays Raw (not editable, preserved).
- Do not use `ParseVTT` for saving. Do not write a literal BOM in Go source (use `"\xef\xbb\xbf"`).

## Verification
`gofmt -l .` empty; `go vet ./...`, `go test ./...` pass. New tests: `vttdoc_test.go` (testdata round-trip, lossy-parser agreement, edge cases, non-vtt rejection, random round-trip, cue parsing/unescape, single-cue edit, empty-payload cases, unwritable edits, byte-level "only one block changed", pair/orig/review names) and `session_test.go` (`TestSessionVTTSaveFlow`: no-op save, edit with `& < >`, backup identical and kept on 2nd save, stale-hash and external-change 409, refused bad edit leaves file alone, sidecar `X.vtt.review.json` without touching `X.review.json`, `.orig.vtt`/`.txt`/non-WEBVTT refused and failed open keeps session; `TestSessionMDStateUnchanged`; `TestDropKind`). `SERMON_PROOF_CORPUS=<N59 folder> go test ./internal/transcript -run Corpus -v`: 69 `.md` and 68 `.vtt` round-trip byte-exact (read only). `node` sanity check of `vttview.js`. Manual in the Browser pane on scratchpad copies: opened `N59_5_10_5_11-ver1.vtt` (21 cues, 08:34 duration, meta in title bar), played from 60 s and the active cue followed, edited a cue with ` & <…>` and saved with ⌘S: `.orig.vtt` created with the original SHA-256, `diff` shows one line changed and escaped; opened the machine draft copy (85 suspect markers listed), 解除 + ⌘S + `C` (sidecar `*.vtt.review.json`), externally modified file -> `/api/save` returned 409. A `.md` copy still loads (`format: md`, no meta). `.orig.vtt` on the command line exits 1.

## Open issues / next steps
- **`.md` and `.vtt` can diverge**: the tool only edits the file you opened and never syncs the other (`X-ver1.md` vs `X-ver1.vtt`). Which one is the single source of truth is the maintainer's decision; not decided here. The `.review.json` sidecars are separate for the same reason.
- `tools/mdtovtt` + `ParseVTT` still exist; `mdtovtt` never overwrites existing `.vtt`.
- No automated JS tests (no npm/test runner by rule); `vttview.js` was checked with `node -e`. A web-only page can reuse `vttview.js` as is.
- Paste of multi-line text into a cue editor is only caught at save time (error names the cue time); could be sanitised in the textarea.
- Windows/Linux file dialog with the two-extension filter is untested on real machines.

## Addendum (2026-11-02, later)
The maintainer decided `.md` stays the single source and `.vtt` is only a derived copy; this feature is kept but not advertised. See `1102e_md_is_source_export_vtt.md` (export button, shared `internal/vttexport`, `mdtovtt -force`). Open issue above about md/vtt divergence is therefore resolved by decision: `.md` wins.
