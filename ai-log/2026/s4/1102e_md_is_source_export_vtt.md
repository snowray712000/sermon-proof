# .md stays the single source; .vtt becomes a derived copy with an export button

- Date: 2026-11-02
- Project: sermon-proof
- Type: feature
- Related logs: 1102c_vtt_open_edit_save_in_editor.md, 1102b_webvtt_intermediate_format_and_converter.md
- Cross-project: `../語音辨識/` pipeline unchanged (still produces `.md`; scheduled command `_prompt_period_using_mac_m1.md` unchanged)
- Commit: uncommitted
- Version: 1.4.0

## Goal / why
Decision by the maintainer (after 1102c): keep the whisper/AI pipeline producing `.md`, keep human proofreading on `.md`, and only add an md -> vtt conversion. `.vtt` need not be usable with the tool. This avoids two sources of truth. Alternative considered and rejected for now: make `.vtt` the single format (drawbacks discussed: one version per file, pipeline/AGENTS.md changes, AI hand-writing valid VTT is error-prone, `status`/review state placement).

## Starting state
1102c made sermon-proof able to open/edit/save `.vtt` (uncommitted). `tools/mdtovtt` held the conversion code and never overwrote existing files.

## What changed (behavior level)
- Conversion moved to a shared package `internal/vttexport` (`FromMD(path, data, Options{Status}) -> Result{Text, Name, Cues, DurationMs, Suspects}`; `NameRe`). Output of the 68 N59 files is byte-identical to the 1102b files (verified by regenerating into scratch and `cmp`).
- `tools/mdtovtt`: new `-force` (regenerate an existing `.vtt`, atomically; identical content reports `same`, changed `NEW`); without it existing targets are still skipped. Still takes `-status` (default `proofread`) and never reads/writes anything but the md and the target.
- sermon-proof UI: button "匯出 .vtt" (shown only when the open file is a `.md`). It asks via `prompt()` which `status` to write (default `proofread`; the user declares it, never inferred), refuses while there are unsaved edits (the export is of the saved file), then `POST /api/export-vtt {status, overwrite}`. 409 `code:"exists"` -> `confirm()` (existing `.vtt` will be replaced; hand edits lost) -> resend with `overwrite:true`. Toasts: exported / updated / "已是最新" (existing file already identical, nothing written).
- The `.vtt` open/edit/save support from 1102c stays in the code but is no longer advertised (README says `.md` is the source; CLAUDE.md records the decision).

## Design and decisions
- `Session.ExportVTT(status, overwrite)`: md sessions only; status trimmed/collapsed, non-empty; reads the md from disk and requires its hash to equal the loaded hash (else `ErrConflict`) so the export matches what the user proofed; target = same folder, name from `vttexport` (`X-ver1.md` -> `X-ver1.vtt`, `X-ver1.orig.md` -> `X-ver1.orig.vtt`; an `.orig.md` always gets `status: machine-draft`); identical -> no write; different and not `overwrite` -> `ErrVTTExists`; write via new `transcript.WriteAtomic` (temp + rename, mode of the md). The md is never written.
- No backup of a replaced `.vtt` (it is regenerable; the user confirms; the machine-draft `X-ver1.orig.vtt` is a different file and is never touched by exporting `X-ver1.md`).
- `FromMD` is lenient for free-form names (no book/passage/speaker/version lines, `status`, `audio` if `X.mp3` exists) instead of failing like the old tool did for explicit files; directory scans in the tool still select only `N*-ver*.md` matching `NameRe`.
- Bug caught on the way: first refactor named both `X-ver1.md` and `X-ver1.orig.md` results `X-ver1.vtt` (they collided); name is now the md name minus `.md` plus `.vtt`.

## Key files / types / functions
`internal/vttexport/vttexport.go` (+test), `internal/server/session.go` (`ExportVTT`, `ExportResult`, `ErrVTTExists`), `server.go` (`handleExportVTT`, route), `internal/transcript/save.go` (`WriteAtomic`), `tools/mdtovtt/main.go`, `web/index.html` (`#btnExport`), `web/app.js` (`exportVtt`).

## Contracts and algorithms
`POST /api/export-vtt` body `{status, overwrite}` -> 200 `{name, path, cues, replaced, unchanged}`; 409 `{code:"exists"|"conflict", error}`; 400 other errors (not md, empty status, no segments). Exported text always satisfies `ParseVTTDoc(text).Render() == text`.

## Gotchas
- `[hidden]` on the button works with the existing button CSS (checked).
- Exporting does not carry `.review.json` ticks; they stay with the md.
- The exported `.vtt` reflects the md at export time; it goes stale after later md edits until exported again (no automatic export on save, by choice).

## Verification
`gofmt -l`, `go vet ./...`, `go test -count=1 ./...` pass; new `internal/vttexport` tests and `TestSessionExportVTT` (not-open, empty status, create, md untouched, unchanged, exists refused, replace, follows a save, refused after external md change, refused from a `.vtt` session). Regeneration of the 68 N59 vtt files: 0 diffs; `-force` rerun: 68 `same`. Browser pane on a scratch copy: button visible for md, export / "已是最新" / confirm-and-replace flows worked, resulting NOTE had the declared status.

## Open issues / next steps
- Nothing writes the `.vtt` automatically after a save; decide if "export on save" is wanted.
- The N59 `.vtt` files in the data folder (from 1102b) are not regenerated here.
- Whether to also expose exporting the `.orig.md` draft in the UI (CLI does it).
- 1102c's read/write `.vtt` support is an unadvertised extra: remove it later if it only adds maintenance, or keep for the planned web page.
