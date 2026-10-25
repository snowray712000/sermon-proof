# Browser seeking drifted up to ~5 s on the pipeline's mp3 files; fixed by serving a synthesized Info frame

- Date: 2026-10-26
- Project: sermon-proof
- Type: fix
- Related logs: 1026b_initial_implementation.md
- Cross-project: `../語音辨識/` (these mp3 files and their timestamps come from there)
- Commit: uncommitted

## Goal / why
Transcript timestamps are true audio times (the pipeline decodes with ffmpeg; the header of `N59_1_12_1_13-ver1.md` says 17:23.722). The tool must make `audio.currentTime` equal true time, otherwise every "jump to segment/suspect" lands in the wrong place.

## Starting state / investigation
`N59_1_12_1_13.mp3`: ID3v2 tag (5888 bytes), then 39955 MPEG1 Layer III frames, 44.1 kHz mono, header bitrate 48 kbps, **every frame 156 bytes with no padding**, no Xing/Info/VBRI tag. True duration = 39955 × 1152 / 44100 = 1043.722 s (matches WebAudio `decodeAudioData`). The browser reported `audio.duration` = 1038.85 s (also what macOS `afinfo` "estimated duration" says): it divides file size by the *nominal* bitrate; real bytes/s is 47.77 kbps, so it is 0.47% short.
Measured in the in-app Chromium: play from `currentTime = T`, capture the output through `MediaElementSource` + `ScriptProcessor`, cross-correlate a block against the fully decoded PCM. Offset (true position − `currentTime`) grew linearly: 0.03 s at T=30, 1.3 at 300, 2.7 at 600, 4.1 at 900, 4.75 at 1030.

## What changed
`GET /api/audio` no longer serves the file verbatim for such files. `internal/audio` builds a "plan": a synthetic first frame followed by the file's audio frames (ID3v2 and trailing junk dropped). The file on disk is never modified.

## Design and decisions
- Plan is built once per file (cached by path+mtime+size), scanning all frame headers (~40k, instant). Pass-through (serve file as is) when: not MP3 Layer III, no frame sync within 4 KB after the ID3 tag, fewer than 100 frames, or the first frame already carries `Xing`/`Info`/`VBRI`.
- **CBR stream** (all frames same bitrate index): tag `Info`, flags = frames|bytes (0x03), **no TOC**. FFmpeg-based decoders (Chromium) then seek linearly from frames/bytes, which is exact for constant frame size.
- **Non-CBR stream**: tag `Xing`, flags frames|bytes|TOC (0x07) with a TOC computed from real frame offsets.
- Rejected: Xing + TOC for the CBR case. First attempt did that and fixed the duration but seeking became noisy (−0.6…−3.8 s random offsets) because TOC entries are bytes/256 (≈4 s resolution for this file).
- Rejected: asking the maintainer to re-encode, or transcoding to WAV (needs external tools, large).
- Tag frame: same MPEG header as the first frame (CRC bit set to "no CRC", padding cleared); if the first frame is too small to hold the tag its (irrelevant) bitrate index is raised. Layout: 4-byte header, zero side info (17 bytes MPEG1 mono / 32 stereo / 9 or 17 MPEG2), tag at offset `4+sideinfo`, then flags, frames (audio frames only), bytes (audio bytes + tag frame length), TOC (100 bytes, Xing only).
- `Source` is an `io.ReadSeeker` over `Head ++ file[Off:Off+N]` so `http.ServeContent` Range requests work.

## Key files
`internal/audio/mp3.go` (`BuildPlan`, `xingFrame`, `Plan`, `Source`, `Cache`), `internal/audio/mp3_test.go`, `internal/server/server.go` (`handleAudio`).

## Verification
After the fix, same measurement at T = 30, 300, 600, 900, 1030: offset constant at −0.09…−0.06 s (output latency) and `audio.duration` = 1043.722449. Unit tests: synthetic CBR (Info, no TOC, audio bytes identical, seeks/ranges), VBR (Xing + monotonic TOC), pass-through cases; `SERMON_PROOF_MP3=<file> go test ./internal/audio -run RealFile -v` prints frames/duration (39955 / 1043.722 s). **Not verified**: Safari/Chrome desktop behaviour with the synthesized tag (only the in-app Chromium pane was measured).

## Open issues / next steps
If a user-facing browser still drifts, check the offset with the same cross-correlation method before changing the tag format.
