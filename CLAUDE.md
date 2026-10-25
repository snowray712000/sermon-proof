# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## 專案定位

**sermon-proof**：本機小工具，讓使用者邊聽音檔邊人工校對講道逐字稿（`.md`）並直接寫回。Go 後端（只用標準庫）+ 原生 HTML/JS/CSS 前端（`go:embed` 打包成單一執行檔），伺服器只綁 `127.0.0.1`，由瀏覽器操作。平台：macOS（Apple Silicon）。

逐字稿由兄弟專案 `../語音辨識/` 的流程產生（見其 `AGENTS.md`）。範例檔：`../語音辨識/信望愛/講道/康來昌/N59_雅各書/`（音檔與 `.md` 同資料夾）。**那個資料夾是使用者的原始資料，唯讀參考，不要修改、不要把工具輸出寫進去。**測試用的樣本複製在 `testdata/`。

## 不可違反的原則

- **忠實**：工具不摘要、不潤稿、不自動改字、不自動標記「已人工確認」。「……」與「...」是停頓或未完成句，不是刪去內容。
- **Round-trip**：未改動就讀入再寫出，內容必須與原檔逐位元組相同（含 CRLF／LF 混用、BOM、結尾有無換行）。只動各段的文字，檔頭、時間戳、表格與不認得的內容一律原樣保留。改動解析／輸出邏輯後必須跑 `go test ./...`。
- **存檔安全**：第一次存檔前複製 `X.orig.md`（之後絕不覆蓋）；先寫同資料夾暫存檔再 `rename`；存檔前比對磁碟檔 hash，被外部改過就拒絕（409）。
- **只改使用者要求的檔案**；修改前先看目標檔案，不要覆蓋未備份的內容。
- **不引入第三方套件**（Go 與前端皆然），要加先問。不用 npm。

## 技術備忘

- Go 已安裝（`/opt/homebrew/bin/go`），不需要 brew。
- 建置：`go build -o sermon-proof .`；測試：`go test ./...`；執行：`./sermon-proof 檔名.md`（無參數則啟動後按「開啟」）。
- 目錄：`internal/transcript`（解析／輸出／配對／存檔／review sidecar）、`internal/audio`（mp3 補 Info/Xing 標頭，修正瀏覽器 seek 偏移，見 `ai-log/2026/s4/1026c_*`）、`internal/filedlg`（osascript 選檔）、`internal/server`（HTTP API）、`web/`（前端，embed）。
- 疑似標記 `[疑似：詞，mm:ss.mmm]` 的 regex 目前只在前端（`web/app.js`）使用，改格式時要同步檢查。

## Git

- 已 `git init`。**只有使用者要求時才 commit / push**。不要 stage 編譯出的執行檔、`.DS_Store`、`.claude/`。
- Commit 訊息用繁體中文簡短描述。

## ai-log（給未來 AI 的開發紀錄，必須遵守）

- 位置：`ai-log/`。規則與範本見 `ai-log/README.md`，索引見 `ai-log/index.md`。（作法沿用使用者的 FHLBible 專案。）
- **開始非瑣碎任務前**：先讀 `ai-log/index.md`，再只讀相關的 log。
- **完成有意義的工作單元後**：寫一份 `ai-log/<年>/s<季>/<MMDD><序號>_<english_title>.md`（s1=1–3 月、s2=4–6、s3=7–9、s4=10–12；序號 a…z、aa、ab…；日期用系統當天），並更新 `index.md`。
- **英文撰寫**；目標是只靠 log 就能大致重現程式（行為、資料格式、API、演算法、決策與踩雷），不是只列改了哪些檔。
- 舊 log 不改寫；不寫入金鑰／密碼；與 `../語音辨識/` 有關的工作用 `Cross-project:` 互相指向。
