# sermon-proof — ai-log index

sermon-proof is a local Go tool (stdlib only, embedded vanilla web UI, loopback server) for proofreading sermon transcripts (`.md`) against their audio and writing the edits back safely. Transcripts come from the sibling project `../語音辨識/`.

Rules for writing and reading logs: [README.md](README.md). Read this index first, then open only the logs relevant to your task.

## 2026 · s4 (Oct–Dec)

| Log | Type | Summary | Areas / keywords | Files touched |
|---|---|---|---|---|
| [1026c_mp3_seek_drift_fix](2026/s4/1026c_mp3_seek_drift_fix.md) | fix | Browsers mis-estimate duration of the header-less 48 kbps mp3s (0.47% short, ~5 s drift at 17 min); audio is now served with a synthesized Info/Xing frame so seeking is exact. | mp3, Xing, Info, TOC, seek drift, `currentTime`, ServeContent, Range | `internal/audio/mp3.go`, `internal/server/server.go` |
| [1026b_initial_implementation](2026/s4/1026b_initial_implementation.md) | feature | First full implementation: parser with byte-exact round-trip, atomic save with `.orig.md`, API, osascript dialogs, web UI, review sidecar, shortcuts. | round-trip, `Parse`/`Render`, save, `.orig.md`, `.review.json`, API, shortcuts, token guard | `main.go`, `internal/*`, `web/*` |
| [1026a_transcript_format_survey_and_design](2026/s4/1026a_transcript_format_survey_and_design.md) | investigation | Survey of the real transcript files (CRLF/LF mix, two timestamp styles, stale checklist table) and the approved design decisions. | format, CRLF, `[疑似：…]`, `## 二、待人工確認清單`, decisions | none (read-only survey) |
