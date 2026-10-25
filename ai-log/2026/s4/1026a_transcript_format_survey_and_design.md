# Transcript format survey and design decisions

- Date: 2026-10-26 (planning discussion started late 2026-10-25)
- Project: sermon-proof
- Type: investigation
- Related logs: 1026b_initial_implementation.md
- Cross-project: `../語音辨識/` (pipeline that produces the transcripts; its `AGENTS.md` defines the content rules)
- Commit: uncommitted

## Goal / why
The maintainer wants a local tool to proofread sermon transcripts (`.md`) while listening to the matching audio, editing the `.md` in place. Before coding, the real files were surveyed so the parser and round-trip guarantee match reality, and the maintainer approved a set of design decisions.

## Starting state
Empty project folder. Go 1.27.3 was already installed (`/opt/homebrew/bin/go`) although the maintainer believed it was not, so no brew install was needed. Samples: `../語音辨識/信望愛/講道/康來昌/N59_雅各書/` (34 mp3, 34 `-ver1.md`, 8 `-ver1-manual.md`, plus a non-transcript `_prompt_period_using_mac_m1.md`). That folder is the maintainer's source data: read-only.

## What was found (facts about the data)
- **Line endings are mixed per file**: ~35 files CRLF, 8 LF. No file mixes both, but the tool must preserve whatever each file uses. No BOM seen (handled anyway).
- **Two segment prefix styles**: `**[mm:ss.mmm]**` (bold, 25 files) and plain `[mm:ss.mmm]` (17 files, including `-manual`). Always one segment per line, blank line between segments, inside `## 一、逐字稿`.
- Time format is always `mm:ss.mmm` (minutes may exceed 59 in principle; the parser also accepts `h:mm:ss.mmm`).
- Suspect marker `[疑似：詞，mm:ss.mmm]` appears inline; the word itself may contain full-width commas (`好，這個忍受呢`), so the regex anchors on the trailing time. Some files (e.g. the manual ones) have none.
- Files end with `## 二、待人工確認清單`, a markdown table (`| 回聽時間（自動） | 疑似內容／範圍 | 不確定之處 |`). It duplicates the inline markers but is NOT updated when text is edited; the tool leaves it untouched and builds its own suspect list from the inline markers.
- Header blocks differ (bullet lists vs `>` quote). Manual files may contain `???` (unknown) and `...` (pause) which are plain text.
- Two `.md` in the folder are not transcripts (zero segments): they must load as all-Raw.
- Audio is 48 kbps mono 44.1 kHz mp3 without a Xing/Info header (see `1026c_mp3_seek_drift_fix.md`).

## Design and decisions (approved by the maintainer: "all as recommended")
1. Go standard library only; front end is vanilla HTML/JS/CSS embedded with `go:embed`; server binds `127.0.0.1` only; macOS file dialog via `osascript`.
2. Round-trip by construction: document = ordered blocks of Raw text and Segments; unchanged segments are emitted from their original bytes.
3. Edit UI: view mode shows highlighted suspects; click a segment to edit in a `<textarea>` (IME-safe, shows the raw markers). Rejected: contenteditable.
4. Enter does not insert a newline in the editor (segments stay single-paragraph); split/merge segments is out of scope for v1 (timestamps).
5. "Reviewed" checkboxes (per segment and per suspect) live in a sidecar `X.review.json`, never in the `.md`, and are never set automatically.
6. A "解除" button turns `[疑似：詞，t]` into `詞` on explicit user action only.
7. Keyboard: browse mode (Space, arrows, R, `[` `]`, Enter, N, C) vs edit mode (Esc = play/pause, ⌘Enter, ⌘↑/↓, ⌘[ ⌘], ⌘S). Browser-reserved and macOS-reserved combos avoided.
8. Jumps start 0.5 s early by default (selectable 0/0.5/1/2) because segment times are auto-aligned and uncalibrated.
9. No autosave to the file; unsaved edits are kept as a draft in `localStorage` keyed by path and file hash.
10. Random port, browser opened automatically (`-no-open`, `-port` flags), stops on Ctrl+C. `git init` done; commits only on request.
11. Backup is `X.orig.md` in the same folder; `.orig.md` files are refused by the opener and skipped in audio pairing.

## Key files
See `1026b_initial_implementation.md`.

## Gotchas
- Do not assume LF. Do not assume `**` around timestamps.
- Do not trust the table at the end of the file to reflect current text.

## Verification
Surveyed with grep/awk over all 43 `.md` files (read-only). Nothing built in this step.

## Open issues / next steps
Read `../語音辨識/AGENTS.md` rules when changing anything about how `...`, `???`, or `疑似` are treated.
