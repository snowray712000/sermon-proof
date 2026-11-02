# Unify N59 transcript file naming (-manual -> .orig.md convention)

- Date: 2026-11-02
- Project: sermon-proof
- Type: config
- Related logs: 1026a_transcript_format_survey_and_design.md, 1026b_initial_implementation.md
- Cross-project: `../語音辨識/AGENTS.md` (naming bullet rewritten); data folder `../語音辨識/信望愛/講道/康來昌/N59_雅各書/`
- Commit: uncommitted (no code in this repo changed)

## Goal / why
The maintainer finished proofreading all 34 N59 sermons, but the files followed two opposite naming schemes. Before any further work (planned: a canonical subtitle-like intermediate format, front end that shows it next to the mp3, feeding it to AI as context) the names had to be made uniform.

## Starting state
- 26 sermons proofread with this tool: `X-ver1.orig.md` = machine draft (backup made by the tool on first save), `X-ver1.md` = proofread latest.
- 8 sermons proofread by hand before the tool existed (`1_1`, `5_7`, `5_10`, `5_12`, `5_13`, `5_15`, `5_17`, `5_19`): `X-ver1.md` = machine draft, `X-ver1-manual.md` = proofread. Same name meant opposite things.
- Every pair has the same segment count. In the 26 tool-proofed files no inline `[疑似：…]` marker remains (a few files only show them inside the stale trailing table).

## What changed (behavior level)
For each of the 8 hand-proofed sermons: `X-ver1.md` -> `X-ver1.orig.md`, then `X-ver1-manual.md` -> `X-ver1.md` (`mv -n`, 16 renames, no content change). Result: 34 `X-ver1.md` (latest) + 34 `X-ver1.orig.md` (machine draft) + 34 mp3 + one non-transcript `_prompt_period_using_mac_m1.md`. SHA-256 of all 16 files compared before/after: identical.

## Design and decisions
- Chose the tool's existing convention as the target because `internal/transcript/save.go` (`OrigPath`) and `pair.go` (`IsOrigName`) already hardcode `.orig.md`; no code change needed. `pair.go` still strips `-manual`, harmless.
- Direction rejected: renaming the 26 to `-manual` (would break the tool's backup logic).
- `../語音辨識/AGENTS.md`: the old `*-manual.md` bullet replaced by a naming bullet (dated 2026-11-02) plus a note that `.transcription_work/` scripts and reference JSONs still say `-manual` as historical records.

## Key files / types / functions
`internal/transcript/save.go` `OrigPath`; `internal/transcript/pair.go` `IsOrigName`, suffix-stripping loop.

## Contracts and algorithms
Naming contract for a sermon base name `N59_a_b_c_d`: `.mp3`, `-ver1.orig.md` (machine draft, never edited), `-ver1.md` (latest proofread; becomes `-ver2…` only if the pipeline reruns).

## Gotchas
- `../語音辨識/.transcription_work/` scripts (`select_*.py`, `build_manual_references.py`) glob `N59_*-ver1-manual.md` and `*manual.md`; they now match nothing. Their outputs (JSON with `manual_file`, `manual_sha256`) still have valid hashes because content did not change, but paths mention the old names. Update the scripts before re-running them.
- Stale `## 二、待人工確認清單` tables remain in all 26 tool-proofed `-ver1.md`; the 8 formerly manual ones have none.

## Verification
Hash comparison of the 16 renamed files before/after = identical; `ls` shows 34/34/34 and zero `*manual*`. No tests run (no code changed).

## Open issues / next steps (not done, to discuss)
Remaining inconsistencies: header disclaimer still says "AI 初稿，尚未逐段人工聽校" in proofread files; 4+ header styles; 19 files bold timestamps vs plain; 8 LF vs CRLF; stray `## N59_5_12_5_12` heading in the former `5_12` manual; stale table. Then design the canonical intermediate format (subtitle-like) for the front end and AI context.
