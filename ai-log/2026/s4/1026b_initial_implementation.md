# Initial implementation of sermon-proof

- Date: 2026-10-26
- Project: sermon-proof
- Type: feature
- Related logs: 1026a_transcript_format_survey_and_design.md, 1026c_mp3_seek_drift_fix.md
- Cross-project: `../語音辨識/`
- Commit: uncommitted

## Goal / why
Implement the approved design: a single Go binary that serves a local web UI for proofreading transcripts against audio.

## What changed (behavior level)
- `sermon-proof [-port N] [-no-open] [file.md [audio.mp3]]`: listens on `127.0.0.1:<port or random>`, prints the URL, opens the browser (`open`). With no file the page shows an empty state; the 「開啟」 button calls `osascript choose file` (server side) and loads the chosen `.md`; 「音檔」 does the same for audio.
- Opening a md pairs audio automatically (see Contracts), loads `X.review.json` if present.
- UI: sticky bar with file name, dirty indicator (`● 未存 N 段`), progress (`已校對 a/b 段 · 存疑已確認 c/d`), 存檔; player row (prev/next segment, ±3 s, play/pause, replay segment, speeds 0.75/1/1.25/1.5/2 with `preservesPitch`, lead-in select, seek bar). Left: segment list (time button, 已校對 checkbox, 還原 button when edited, text with `<mark class="sus">` suspects). Right: suspect list (checkbox, time, text, 解除), 「只顯示未確認」 filter, shortcut help.
- Active segment follows playback (binary search by start time, rAF loop while playing), auto-scrolls only if out of view. After a jump the segment is "pinned" as current until playback passes its start, so lead-in seconds don't highlight the previous segment.
- Saving (⌘S / button): sends only changed segments; server applies, renders, backs up, atomically writes, returns fresh state. 409 on external change → confirm to reload (discarding edits).
- Last playback position per file and speed/lead-in remembered in `localStorage`.

## Key files / types / functions
- `main.go`: flags, listener, browser open.
- `internal/transcript/doc.go`: `Parse`, `Document.Apply/Render/Clone`, `Block`, `segRe`.
- `internal/transcript/pair.go`: `PairAudio`, `IsOrigName`.
- `internal/transcript/save.go`: `Save` (backup + atomic write), `Hash`, `OrigPath`, `ReviewPath`, `Review`, `LoadReview`, `SaveReview`.
- `internal/server/session.go`: `Session` (one open file; mutex), `State`, `Seg`, `Edit`.
- `internal/server/server.go`: routes, host/origin/token guard, audio handler.
- `internal/audio/mp3.go`: serves mp3 with a synthesized Info/Xing frame (see log 1026c).
- `internal/filedlg/filedlg.go`: `osascript` dialogs (`{"md"}`, `{"mp3","m4a","wav"}`); cancel = stderr containing `-128`.
- `web/{index.html,app.js,style.css,embed.go}`: front end, embedded.
- Tests: `internal/transcript/doc_test.go` (round-trip on `testdata/*.md`, edge cases, 3000 random inputs, optional corpus via `SERMON_PROOF_CORPUS=<folder>`), `internal/audio/mp3_test.go`, `internal/server/session_test.go`.

## Contracts and algorithms
**Segment line regex**: `^((?:\*\*)?\[(\d{1,3}(?::\d{1,3}){1,2}\.\d{3})\](?:\*\*)?)([ \t　]*)(.*)$`. Continuation lines: lines directly after a segment line that are non-blank, not a segment, not starting with `#`, `|`, `>`, `- `, `* `, or a code fence, are part of the same segment (their EOLs stay inside `Body`).
**Round-trip**: lines split on `\n`, a preceding `\r` is part of the EOL. Non-segment lines accumulate into one Raw block. Render emits `Raw` for every block whose `Body == OrigBody`; edited segments are re-emitted as `Prefix + Sep + Body + EOL` (Sep defaults to one space if it was empty). New text's `\r\n`/`\r`/`\n` are normalised to the document's dominant EOL (CRLF if more than half the lines are CRLF).
**Audio pairing**: strip `.md`; try `<base>.mp3` / `.MP3`; then strip one of `.orig`, `-manual`, `-verN` and retry, until nothing is left to strip.
**Save**: read disk; sha256 must equal the hash loaded earlier (else `ErrConflict`); create `X.orig.md` with `O_EXCL` (existing backup untouched, a backup failure aborts the save); write `.X.md.tmp-*` in the same dir, `chmod` to the original mode, `fsync`, `rename`. A save with zero effective changes writes nothing.
**Review sidecar** `X.review.json`: `{"segments": {"<startMs>": true}, "suspects": {"<markerMs>": true}}`.
**Suspect regex (front end only)**: `/\[疑似[：:]([\s\S]*?)[，,](\d+(?::\d{1,3}){1,2}\.\d{3})\]/g`.
**HTTP API** (all `/api/*` need header `X-Token` or `?t=` for `<audio>`; non-GET needs `Content-Type: application/json`; Host must be `127.0.0.1:<port>` or `localhost:<port>`; Origin, if present, must equal `http://<host>`): `GET /api/state`; `POST /api/pick-md`, `/api/pick-audio` (dialog then load; `{"cancelled":true}` on cancel); `POST /api/reload`; `POST /api/save {baseHash, edits:[{id,text}]}` → state or 409 `{code:"conflict"}`; `PUT /api/review`; `GET /api/audio` (Range supported). The client never sends file paths; they only come from the CLI or the native dialog. The token is generated at start and substituted into `index.html` (`__TOKEN__`).
**State JSON**: `{open, mdPath, mdName, audioPath, audioName, hasAudio, hasOrig, hash, eol:"lf"|"crlf", segments:[{id,startMs,time,body}], review}`; `body` uses `\n` for internal line breaks.

## Gotchas
- Browser buttons keep focus after a click and would swallow Space/arrow shortcuts: a document click handler blurs buttons/checkboxes/range/select.
- IME: all editor key handling returns early on `isComposing`/keyCode 229.
- The Go source for tests must not contain a literal BOM character (use `\xef\xbb\xbf`).
- Go source files in this repo were written through a tool that turns `﻿` escapes into the actual character; check with `gofmt -l`.

## Verification
- `go vet ./...` clean; `go test ./...` passes, including round-trip on all 43 real `.md` files (`SERMON_PROOF_CORPUS`).
- Manually in the in-app browser against a scratch copy: layout, suspect highlighting, editing, ⌘S (diff vs `.orig.md` shows exactly the one edited line, CRLF kept), `.orig.md` identical to the original, N / Space shortcuts, wrap-around to first suspect.
- curl: 403 for wrong Host / Origin / token, 415 without JSON content type, 409 after external modification, 206 for Range.
- `osacompile` syntax check of both dialog scripts. **Not verified**: the native file dialogs themselves (need a human), Safari/Chrome behaviour (only the in-app Chromium pane was used), the 解除 button, the draft-restore prompt and the conflict confirm dialog in the UI.

## Open issues / next steps
- Not committed.
- Splitting/merging segments, undo beyond per-segment 還原, and regenerating the stale `## 二` table are intentionally not implemented.
