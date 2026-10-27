# Windows drag-and-drop failed because nothing could locate the dropped file; added a wide home/drive walk

- Date: 2026-10-27
- Project: sermon-proof
- Type: fix
- Related logs: 1026d_drag_drop_open.md, 1027a_cross_platform_release_and_npm.md
- Commit: uncommitted

## Symptom
On Windows, dragging a `.md` / audio onto the page failed ("找不到這個檔案的位置…") while the 開啟 / 音檔 dialog buttons worked. User suspected a `/` vs `\` path mismatch.

## Diagnosis
No separator bug found (`uriPath` already converts `file:///C:/x` → `C:\x`; walk uses `filepath.Separator`). Real cause: `locate.Find` has three strategies and none works on a fresh Windows session — Chrome/Edge give no `text/uri-list`, `mdfind` is macOS-only, and the folder walk only covers the folders of an already-open transcript/audio (empty at start). Not reproduced on a real Windows machine (none available); verified by reasoning + cross-compile.

## Changes (`internal/locate/locate.go`)
- New last-resort step `walkWide(name, add)` after the open-folder walk: roots from `wideRoots()` = user home, plus on Windows every existing drive root `C:\`…`Z:\`. Depth 6, 20 s overall deadline, stops at the first root that yields a match (so identical copies inside one root still report ambiguity). Same size + mtime(±1 ms) acceptance rule as before.
- `walkTree(root, name, depth, deadline, add)` replaces the inline walk. For wide walks `skipDir` prunes hidden/`$`-prefixed folders and `AppData`, `node_modules`, `Windows`, `Program Files*`, `ProgramData`, `System Volume Information`, `Library`. Walk errors on a file no longer `SkipDir` the whole folder.
- `uriPath`: Windows UNC (`file://server/share/x` → `\\server\share\x`), and `filepath.FromSlash` before `Clean`.
- `wideRoots` is a package var; the test file's `init` sets it to nil so other tests never scan the real home folder (that made the suite take 17 s). `TestFindByWideWalkWhenNothingIsOpen` covers the new path, hidden-folder skipping, and size mismatch.

## Caveats
Slow on very large drives (bounded by the 20 s deadline); a transcript deeper than 6 levels, or on a drive/path not scanned in time, still needs the dialog buttons. Windows behaviour is untested on a real machine.
