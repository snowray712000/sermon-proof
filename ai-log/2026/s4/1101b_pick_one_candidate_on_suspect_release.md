# Pick one candidate when releasing a multi-option suspect marker

Date: 2026-11-01 · Type: feature · Files: `web/app.js`, `web/style.css`

## Problem
Markers can carry several candidate wordings separated by a full-width slash, e.g. `[疑似：決志／覺知，00:33.580]`. The 解除 button replaced the marker with the whole inner text (`決志／覺知`), so the user then had to edit the segment by hand to delete the unwanted candidate.

## Behavior
- Frontend only. No Go / format change; the marker regex (`SUSPECT_RE`) is untouched.
- `optionsOf(word)` splits the marker's inner text on `／` (U+FF0F only; ASCII `/` is not treated as a separator) and drops empty pieces.
- In the right-hand suspect list, when a marker has 2+ options, a row of small buttons (`.opts`, one per candidate) is added under the item (grid columns 2–3). Clicking one calls `removeSuspect(s, option)`, which replaces the whole `[疑似：…，時間]` with exactly that candidate.
- The existing 解除 button is kept and still substitutes the full text (all candidates), for the case where the user wants to keep everything; its tooltip says so when there are several options.
- `removeSuspect(s, replacement = s.text)`: same edit path as before (`setText`, refresh open textarea, `renderBody`, `refreshAll`), so the segment becomes "edited" and still needs 存檔. The "已確認" checkbox is NOT auto-ticked (project rule: never auto-confirm).
- CSS: `#suspects .opts` (flex wrap, grid-column 2 / 4) and `#suspects .opt` (font 14px).

## Verification
Served a copy of `testdata/N59_1_12_1_13-ver1.md` from the scratchpad, clicked a candidate via the page: marker `試人／試神` disappeared from the list, dirty indicator showed `● 未存 1 段`. `go build` and `go test ./...` pass (no transcript code changed). Not checked: candidate buttons for 3+ options on a narrow window, keyboard-only use.
