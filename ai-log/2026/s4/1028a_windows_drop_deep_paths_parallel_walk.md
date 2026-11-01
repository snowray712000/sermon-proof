# Windows drag-and-drop still failed for deep folders; widened depth and made the wide walk parallel

- Date: 2026-10-28
- Project: sermon-proof
- Type: fix
- Related logs: 1027b_windows_drop_locate_fallback.md, 1026d_drag_drop_open.md
- Cross-project: sample data lives in `../語音辨識/信望愛/講道/康來昌/N59_雅各書/` (read-only; only read here)
- Commit: uncommitted
- Supersedes / Superseded by: refines 1027b (depth/deadline constants and walkWide shape)

## Goal / why
After 1027b, drag-and-drop on a real Windows machine (project pulled from the Mac mini, dist build) still failed with "找不到這個檔案的位置…". The user confirmed the files sit in `D:\coding\信望愛\語音辨識\信望愛\講道\康來昌\N59_雅各書`.

## Starting state
`locate.Find` order: file:// URIs → Spotlight (macOS only) → walk of open-file folders (`walkDepth` 3) → `walkWide` over home + every drive root, `wideDepth` 6, one shared 20 s deadline, roots searched sequentially.

## What changed (behavior level)
- Root cause: that folder is 7 levels below `D:\` (coding, 信望愛, 語音辨識, 信望愛, 講道, 康來昌, N59_雅各書) but `wideDepth` was 6, so the file was never visited. Reproduced with a throwaway test calling `Find` on real files in that folder with `wideRoots` = home, `C:\`, `D:\`.
- `wideDepth` 6 → 12.
- Sequential walk with the deeper limit took 10–33 s (home, then C:, then D:). `walkWide` now starts one goroutine per root, each with its own `wideDeadline` (now 15 s per root, so a huge C: cannot starve D:). Hits go through a mutex-guarded `add`; the first hit sets an `atomic.Bool` stop flag that every `walkTree` checks and answers with `filepath.SkipAll`. Measured ~5–6 s per dropped file on the user's machine (.md and .mp3 both found).
- `walkTree` gained a `stop *atomic.Bool` parameter (nil for the open-folder `walk`).

## Design and decisions
- Ambiguity detection across roots is weaker now: once one root hits, the others stop, so identical copies on two different drives are not reported as ambiguous (copies inside one root still are). Accepted for speed.
- Rejected for now: remembering recently opened folders to search first (would make it near-instant); offered to the user as a possible follow-up.

## Key files / types / functions
`internal/locate/locate.go`: `walkWide`, `walkTree`, `wideDepth`, `wideDeadline`.

## Contracts and algorithms
Match rule unchanged: same base name, same size, mtime within ±1 ms. `skipDir` pruning (hidden, `$`-prefixed, AppData, Windows, Program Files*, node_modules, ProgramData, System Volume Information, Library) applies to wide walks only.

## Gotchas
- A path with N folders below the drive root needs `wideDepth >= N`; deep research-data trees exceed small limits easily.
- `TestFindByURIAndAmbiguity` failed on Windows because the test built the `file://` URL from `C:\…` directly; it now converts with `filepath.ToSlash` and a leading `/` (product code `uriPath` was already correct).
- Tests keep `wideRoots` nil via `init` so the suite never scans the real disk.

## Verification
User confirmed drag-and-drop works with a freshly built binary. `go test ./...` passes except `internal/transcript` `TestSaveBackupAtomicAndConflict`, which asserts Unix permission bits (`mode -rw-rw-rw-` on Windows) — pre-existing, unrelated, not fixed.

## Open issues / next steps
- ~5 s delay per drop; a recent-folders-first search would remove it.
- `TestSaveBackupAtomicAndConflict` should skip the mode check on Windows.
- Build on Windows: `go build -o sermon-proof.exe .` (or `go run ./tools/release`, which cross-builds each target itself via `go build` with `GOOS`/`GOARCH`).
