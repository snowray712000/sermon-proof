# Edit the candidate wording in the side list before releasing a suspect marker

Date: 2026-11-01 · Type: feature · Files: `web/app.js`, `web/style.css`

## Problem
When none of the recognizer's candidates in a `[疑似：詞，mm:ss.mmm]` marker was right, the user had to 解除 first (marker becomes plain text in the segment on the left) and then scroll to that segment and fix the wording by hand. They wanted to correct the candidate text directly in the right-hand list, then release.

## Behavior
- Frontend only; no Go / format change. `SUSPECT_RE` untouched.
- In each right-hand suspect item, the candidate text is now an `<input type="text" class="word">` prefilled with the marker's inner text (e.g. `決志／覺知`, all candidates). The user edits it freely (delete a candidate, type a new word, empty it).
- 解除 button and Enter inside the field (ignored while IME `isComposing`) call `removeSuspect(s, word.value)`, replacing the whole `[疑似：…，時間]` with the field's text. Same edit path as before (`setText` → segment becomes edited, needs 存檔; 已確認 is NOT auto-ticked).
- The per-candidate buttons from 1101b are unchanged (click = replace immediately with that candidate, ignoring the field).
- Drafts: `renderSuspects()` rebuilds the list on every `refreshAll()` (e.g. ticking another checkbox), which would wipe typed text. `suspectDrafts` (Map keyed by suspect `ms`, same key as `review.suspects`) stores the field's value on `input`; render prefills from it; `removeSuspect` deletes the entry and `adopt()` clears the map when a file is opened. The draft is in memory only (not persisted).
- Field `click` stops propagation so it does not jump the audio; the global shortcut handler already ignores `INPUT` targets, so typing does not trigger space/arrow shortcuts.
- CSS: `#suspects .st {min-width:0}`, `#suspects .word` full width, inherited font, 15px.

## Verification
Served a scratchpad copy of `testdata/N59_1_12_1_13-ver1.md`; via the page changed a two-candidate field to `決志`, toggled the 只顯示未確認 filter (re-render kept the typed value), clicked 解除: list went 85 → 84, dirty indicator `● 未存 1 段`, segment text contained the new wording. `go test ./...` passes. Not checked: IME composition + Enter in a real browser, narrow window layout.
