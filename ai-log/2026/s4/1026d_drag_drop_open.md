# Drag-and-drop opening of transcripts and audio; the server identifies the file because browsers hide dropped paths

- Date: 2026-10-26
- Project: sermon-proof
- Type: feature
- Related logs: 1026b_initial_implementation.md
- Commit: uncommitted

## Goal / why
User asked to be able to drag files into the window instead of using the osascript "開啟" / "音檔" dialogs.

## Key constraint
A web page never gets the real path of a dropped file (`File` exposes only `name`, `size`, `lastModified`). The tool must write back to the original location, so uploading bytes is not an option (saving would land elsewhere). Decision: the browser sends name/size/mtime and the **server finds the file**.

## Design
- `internal/locate` (`Find(Query, dirs) (path, error)`): candidates come from, in order, stopping at the first step that yields a match:
  1. `file://` URIs from `dataTransfer.getData('text/uri-list')` (Safari supplies them; Chrome does not) whose basename equals `Name`;
  2. Spotlight `mdfind -name <Name>` (5 s timeout), keeping results whose basename equals `Name` exactly (mdfind is substring);
  3. a depth-3 `WalkDir` of the folders of the currently open md / audio and of their parents (Spotlight may exclude folders).
  A candidate is accepted only if size equals and mtime is within ±1 ms of `File.lastModified`. Zero matches → `ErrNotFound` (message tells the user to use the buttons); several matches (identical copies) → ambiguity error, nothing opened. No normalization of NFC/NFD names (no third-party packages).
- `POST /api/open-dropped` body `{files:[{name,size,mtimeMs}], uris:[...]}` (JSON + token like all `/api/*`). Extension decides kind: `.md` → transcript, `.mp3/.m4a/.wav` → audio, anything else → 400. At most one of each. The md is opened first via `Session.OpenMD` (which auto-pairs audio), then a dropped audio goes through `Session.SetAudio`. Audio alone requires a transcript already open. Returns the usual `State`. `Session.SearchDirs()` supplies the walk folders. `.orig.md` is still refused by `OpenMD`.
- Front end (`web/app.js`, "drag & drop" section): document-level `dragenter/dragover/dragleave/drop`, only for drags whose types include `Files`; `preventDefault` on dragover/drop is essential or the browser navigates to / plays the file. `body.dropping` shows a dashed overlay (`style.css`, `::after`). If a `.md` is dropped, `guardDirty()` runs first (unsaved-edit confirm) and the result goes through `adopt()`; audio-only drop keeps unsaved edits (`S = st; setAudio()`, same as `pickAudio`). Errors → `alert`.
- Round-trip/save invariants untouched: drop only reads.

## Verification
`go test ./...` (new `internal/locate/locate_test.go`: size+mtime filtering, URI path, ambiguity). Manual curl against a built binary: valid testdata md opened (state returned), wrong size → not-found message, `.txt` → unsupported message. The actual browser drag gesture was not exercised in a real browser.
