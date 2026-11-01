# Dropped-file lookup: search recently opened folders first, and stop short-circuiting the wide walk

- Date: 2026-11-01
- Project: sermon-proof
- Type: feature
- Related logs: 1028a_windows_drop_deep_paths_parallel_walk.md, 1027b_windows_drop_locate_fallback.md, 1026d_drag_drop_open.md
- Commit: uncommitted
- Supersedes / Superseded by: refines 1028a (the early-stop flag is removed)

## Goal / why
1028a left two known weaknesses: every drop on Windows/Linux took ~5 s (wide walk of home + drives), and identical copies on two different drives were not reported as ambiguous because the first hit stopped all other roots. Picking the wrong copy would make the user proofread and save into a different file than the one they dragged.

## What changed (behavior level)
- New `internal/locate/recent.go`: an MRU list (max 20) of folders of files opened before. `Remember(path)` stores `filepath.Dir(path)` newest-first, de-duplicated; `SetRecentFile(path)` loads that JSON array and makes every later `Remember` rewrite it (temp file + rename, all errors ignored — it is only a speed-up). Without `SetRecentFile` the list is in-memory only, so tests never touch the real config dir.
- `Find` stage order is now: file:// URIs → Spotlight (macOS) → folders of the open transcript/audio → **recent folders** → wide walk. Each recent folder goes through the same `walk` as the open folders (the folder and its parent, depth 3), so a sibling audio folder is covered. The first stage with any match wins.
- `walkWide` no longer stops on the first hit: every root runs to completion or its own 15 s `wideDeadline`, so copies on different roots all reach `add` and `Find` returns the "有多個一模一樣的檔案" error. `walkTree` lost its `stop *atomic.Bool` parameter.
- `add` de-duplicates with `os.SameFile` instead of string equality, because overlapping roots (home inside `C:\`, or `.`-spelled paths) reach one file by different spellings and must not count as two copies. Paths that cannot be stat'ed are dropped.
- `Session.OpenMD` (md and the paired audio) and `Session.SetAudio` call `locate.Remember`, so files opened by dialog, CLI argument or drop all feed the list. `main.go` calls `locate.SetRecentFile(<UserConfigDir>/sermon-proof/recent-dirs.json)` before opening CLI arguments. Locations: macOS `~/Library/Application Support/sermon-proof/`, Windows `%AppData%\sermon-proof\`, Linux `~/.config/sermon-proof/`.

## Design and decisions
- Trade-off accepted: a hit in a recent folder (like a hit in an open folder, or a URI) ends the search, so an identical copy on another drive is not detected in that case. Only the wide walk, used when nothing else hits, checks all roots. Making recents also scan everything would bring the 5+ s back on every drop.
- Cost: the first ever drop (empty list, nothing open) and any drop of a never-opened folder still run the full wide walk, now up to ~15 s (the slowest root's deadline) instead of ~5 s, since it no longer exits on the first hit. Every file opened afterwards makes later drops of that folder instant.
- The list stores folders, not files, and stale folders are harmless (`WalkDir` errors are skipped).

## Key files / types / functions
`internal/locate/recent.go` (`Remember`, `SetRecentFile`, `recentDirs`, `maxRecent`), `internal/locate/locate.go` (`Find`, `add`, `walkWide`, `walkTree`), `internal/server/session.go` (`OpenMD`, `SetAudio`), `main.go`.

## Gotchas
- `Remember` is called while `Session.mu` is held in `OpenMD`; it only takes `recent.mu`, so there is no lock-order problem, but do not call back into `Session` from it.
- Tests that touch the list must reset the package variable `recent` (see `resetRecent` in `locate_test.go`); the recent-folder test makes `wideRoots` fail the test if it is called, proving the wide walk is skipped.

## Verification
`go vet ./...`, `go test ./...` (new: recent-first without wide walk, persistence/ordering/cap, copies on two roots ambiguous, overlapping roots counted once), and `GOOS=windows` / `GOOS=linux` builds. Not measured on a real Windows machine yet: expected first drop slower (~15 s worst case), later drops of already-opened folders instant.

## Open issues / next steps
- Confirm timing on the user's Windows machine; if the cold first drop is too slow, lower `wideDeadline` or give recents a bounded extra duplicate check.
- `TestSaveBackupAtomicAndConflict` still asserts Unix permission bits (fails on Windows, pre-existing).
