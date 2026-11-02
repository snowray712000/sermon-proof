# WebVTT as the canonical sermon transcript format + md->vtt converter

- Date: 2026-11-02
- Project: sermon-proof
- Type: feature
- Related logs: 1102a_unify_n59_file_naming.md, 1026a_transcript_format_survey_and_design.md, 1026b_initial_implementation.md
- Cross-project: `../語音辨識/信望愛/講道/康來昌/N59_雅各書/` (source data, read only in this step)
- Commit: uncommitted

## Goal / why
The maintainer wants one common, subtitle-like intermediate format for finished transcripts so that a later web front end can show text next to the mp3, users can submit updates, and AI can read the transcript with background context. Decision (approved): **WebVTT, one `.vtt` per transcript**, produced 1:1 from the existing segments. The `.md` files stay as they are for now; the sermon-proof editor does not read `.vtt` yet.

## Starting state
All 34 N59 sermons have `X-ver1.md` (proofread) and `X-ver1.orig.md` (machine draft) after 1102a. Segments carry only a start time; headers/tables are free-form and inconsistent.

## What changed (behavior level)
- `internal/audio/duration.go`: `Duration(path)` sums MP3 frame samples (skips a leading Xing/Info frame).
- `internal/transcript/vtt.go`: `VTT{Meta []KV, Cues []Cue}`, `FromDocument`, `(*VTT).String()`, `ParseVTT`, `FormatVTTTime`.
- `tools/mdtovtt`: `go run ./tools/mdtovtt -out DIR FOLDER_OR_FILE...` converts every `N59_a_b_c_d-verN.md` and `-verN.orig.md`; never overwrites (O_EXCL, reports SKIP); parses each result back and compares start + text with the source segments before writing.
- Run on the 34 sermons into a scratch dir: 68 files, 0 failures, durations match the md headers (±1 ms).

## Design and decisions
File layout (`\n` line endings, trailing newline):

```
WEBVTT

NOTE
book: 雅各書 (N59)
passage: 5:10–11
speaker: 康來昌
audio: N59_5_10_5_11.mp3
duration: 08:34.324
version: ver1
status: proofread | machine-draft
draft: N59_5_10_5_11-ver1.orig.vtt      (final files only)
timing: 每段開始時間為自動對齊，未經人工校準；結束時間為下一段開始時間
notation: 「……」與「...」表示停頓、轉折或未完成句，不是刪節[；「???」表示未知]
converted-from: N59_5_10_5_11-ver1.md

s001
00:00.000 --> 00:09.680
text on one line (or several lines if the md segment had continuation lines)
```
- One cue per md segment; id `sNNN` (1-based, in order). End = next cue's start; last cue ends at audio duration; if that is not after the start, start+1000 ms.
- Times `mm:ss.mmm`, `hh:mm:ss.mmm` from one hour. `ParseTimeMs` is reused for reading.
- Cue text is the segment body, trimmed, CRLF -> LF. Payload escapes `&`, `<`, `>` as `&amp; &lt; &gt;` (so `-->` can never appear in a payload); the parser reverses it. Source data had none of these characters.
- Metadata is the first `NOTE` block, `key: value` lines (players ignore it). Derived: book code from filename `N59_a_b_c_d`; book name = parent folder `N59_雅各書`; speaker = grandparent folder; passage = `a:b`, `a:b–d` (same chapter) or `a:b–c:d`.
- `status: proofread` for `-verN.md` comes from the maintainer's explicit statement that all 34 are finished (flag `-status`); the tool never infers it, in keeping with "never auto-mark as human-confirmed". Machine drafts get `machine-draft`.
- Dropped on purpose (still in the `.md`/`.orig.md`): per-file method/model header, stale `## 二、待人工確認清單` table, old "尚未逐段人工聽校" disclaimer. Inline `[疑似：詞，t]` markers (only in machine drafts) are kept verbatim in cue text.
- Machine draft backup name by existing rule `OrigPath`: `X-ver1.orig.vtt` (works for any extension).

## Key files / types / functions
`internal/transcript/vtt.go`, `vtt_test.go`; `internal/audio/duration.go` (+ `TestDuration` in `mp3_test.go`); `tools/mdtovtt/main.go`.

## Contracts and algorithms
Round-trip guarantee for the conversion is semantic (start ms + trimmed text per segment), not byte-exact; the `.md` is not replaced. `ParseVTT(String(v)).String()` is stable (tested on `testdata/*.md`).

## Gotchas
- The Write tool turned a `"﻿"` Go literal into a raw BOM and broke compilation; use `"\xef\xbb\xbf"`.
- `4_7`, `4_11`, `4_13` `-ver1.md` still mention `[疑似` in the stale table only; their cues have none.
- Last cue end depends on the mp3 being next to the md; without it the duration/audio fields are omitted and the last cue gets start+1000 ms.

## Verification
`go vet ./...`, `go test ./...` pass (new: `TestVTTFromTestdata`, `TestVTTEscapeAndMultiline`, `TestFormatVTTTime`, `TestParseVTTRejectsNonVTT`, `TestDuration`). Converter run on all 34 sermons -> 68 `.vtt` in the session scratchpad (not yet in the data folder); 0 final files contain `疑似`.

## Open issues / next steps
- Decide whether to write the `.vtt` files into the sermon folder (not done; that folder is the maintainer's data).
- Teach sermon-proof to open/save `.vtt` (cue = segment; keep "only change text" and `.orig` backup; review sidecar keyed by cue id).
- Remaining md inconsistencies are moot for the vtt but still exist in the `.md` files.
- Front end: `<audio>` + `TextTrack` (`mode = "hidden"`, `cuechange`) for highlight/seek.

## Addendum (2026-11-02, same day)
The maintainer approved writing the output into the data folder. `go run ./tools/mdtovtt -out <N59_雅各書> <N59_雅各書>` created 68 new `.vtt` files there (nothing pre-existing, no overwrites); they are byte-identical to the scratch run. The folder now holds, per sermon: `.mp3`, `-ver1.md`, `-ver1.orig.md`, `-ver1.vtt`, `-ver1.orig.vtt`. The `.md` files were kept.
