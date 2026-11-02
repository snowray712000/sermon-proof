# Switch between sibling transcripts from a side list

- Date: 2026-11-02
- Project: sermon-proof
- Type: feature
- Related logs: 1102c_vtt_open_edit_save_in_editor, 1026d_drag_drop_open
- Cross-project: none
- Commit: uncommitted (on top of the uncommitted 1102c work)

## Goal / why
A sermon folder holds dozens of `.md`/`.vtt` + `.mp3`. After opening one, the user had to drag or press 「開啟」 for every next file. Now the open file's folder is listed in the sidebar and a click switches.

## Starting state
Open paths: `/api/pick-md` (OS dialog), `/api/open-dropped`, CLI arg. All end in `Session.Open(path)`, which pairs audio via `transcript.PairAudio` and the front end calls `adopt(state)`.

## What changed (behavior level)
- Sidebar (above 存疑處) shows a collapsible 「同資料夾逐字稿 (N)」 list: every `.md`/`.vtt` in the folder of the open transcript, natural-sorted (`localeCompare(..., {numeric:true})`). Current one is highlighted. Hidden (`hidden`) when the folder has fewer than 2 transcripts or nothing is open.
- Tags per row: 「無音檔」 (orange, no paired mp3) and 「已存過」 (`.orig.<ext>` exists).
- Click a non-current row: dirty guard (`confirm` if unsaved edits) → pause audio → switch → `adopt`. Toast if no audio is paired.
- Audio-only files (mp3 without transcript) are not listed: nothing to proofread.

## Design and decisions
- List only transcripts; audio is derived by `PairAudio` per file, so one list covers both "mp3 or md" needs.
- Server never takes a path from the client: `POST /api/open-sibling {name}` accepts a bare file name only (`name == filepath.Base(name)`), must pass `isSiblingName`, and is joined to the *currently open* file's folder. No arbitrary file read.
- Excluded from the list: dirs, names starting with `.` (macOS `._X.md` AppleDouble files on external drives), `.orig.md`/`.orig.vtt`, sidecars (`.review.json` is not `.md`/`.vtt`).
- Separate GET endpoint instead of adding siblings to `State` (State is returned by every save).
- No per-file review progress in the list (would need reading each sidecar); possible later.

## Key files / types / functions
- `internal/server/session.go`: `Sibling{Name,HasAudio,HasOrig,Current}`, `isSiblingName`, `Session.Siblings()`, `Session.OpenSibling(name)`.
- `internal/server/server.go`: `GET /api/siblings` → `[]Sibling`; `POST /api/open-sibling` → `State` (400 with message on error).
- `web/app.js`: `refreshFiles()` (called at end of `adopt`; `filesSeq` drops stale responses), `openSibling(name)`.
- `web/index.html`, `web/style.css`: `#files`, `#fileList`.
- `internal/version`: 1.5.0.

## Contracts and algorithms
`Siblings()`: read `filepath.Dir(mdPath)`, keep non-dir entries passing `isSiblingName`; per entry `PairAudio` (stat of mp3 candidates) and `os.Stat(OrigPath)`. Order is directory order; the UI sorts.

## Gotchas
- `openSibling` must `savePos(); audio.pause()` *before* the request: the `pause` handler calls `savePos()` which keys on `S.mdPath`; after `adopt` replaced `S` the position would be stored under the new file (same latent issue exists in `pickMd`, not touched).
- `HasOrig` only reflects the same extension (`X.md` ↔ `X.orig.md`), so an `.md` with only `.orig.vtt` next to it shows no 「已存過」.

## Verification
`go vet ./... && go test ./...` incl. `TestSessionSiblings` (listing, filters, traversal / backup / hidden names refused, switch, not-open error). Manually: ran the binary on a scratch folder, clicked a row in the Browser pane, file name and highlight changed.

## Open issues / next steps
- Keyboard shortcut for previous/next file (not added; needs a key that does not clash with ↑/↓ segment navigation).
- Show proofreading progress per file; list audio-only files greyed out.
- Not committed.
