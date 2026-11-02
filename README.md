# sermon-proof

邊聽音檔邊人工校對講道逐字稿，直接修改 `.md`。Go 標準庫 + 原生 HTML/JS/CSS，單一執行檔，只綁 `127.0.0.1`。

## 安裝

發行檔提供 Windows x64、macOS Apple Silicon、Linux x64。

```bash
npm i -g @fhlnet/sermon-proof     # 需要 Node.js 16+，npm 會自動挑對應系統的執行檔
sermon-proof 檔名.md
```

或到 GitHub Release 下載壓縮檔（`sermon-proof_<版本>_<windows|darwin|linux>_<amd64|arm64>`，附 `SHA256SUMS.txt`），解壓後直接執行。目前沒有數位簽章與 Apple 公證，第一次執行 macOS（Gatekeeper）或 Windows（SmartScreen）可能跳警告。

選檔視窗：macOS 用 osascript，Windows 用 PowerShell，Linux 需要 `zenity` 或 `kdialog`（沒有就改用拖曳或命令列帶檔名）。

## 建置與執行

```bash
go build -o sermon-proof .
./sermon-proof /path/to/N59_1_12_1_13-ver1.md      # 自動配對同名 .mp3
./sermon-proof                                     # 不帶參數，按頁面上的「開啟」選檔
```

選項：`-port N`（預設隨機）、`-no-open`（不自動開瀏覽器）。第二個參數可手動指定音檔。結束用 Ctrl+C。

## 它做什麼、不做什麼

- 只改各段的文字；檔頭、時間戳、表格與不認得的內容原樣保留（未改動就存檔，內容與原檔逐位元組相同，含 CRLF／LF）。
- 第一次存檔前複製 `X.orig.md`（之後不覆蓋）；先寫暫存檔再改名；磁碟檔被外部改過就拒絕存檔。
- 「已校對」「已確認」勾選記在 `X.review.json`，不寫進 `.md`，也不會自動勾。
- 「匯出 .vtt」按鈕（或 `go run ./tools/mdtovtt -force -out 資料夾 資料夾`）由 `.md` 產生同名的 WebVTT 副本；`.md` 是唯一來源，`.vtt` 只是衍生檔，重新匯出會取代它，不會回頭改 `.md`。
- 檔尾的 `## 二、待人工確認清單` 表格不會隨修改更新；側邊「存疑處」清單以行內 `[疑似：…]` 標記為準。
- mp3 若沒有 Xing/Info 標頭，瀏覽器會把時間估算偏短（最多約 5 秒）；工具提供音檔時會補上標頭，不動原檔。

## 快捷鍵

頁面右側「快捷鍵」有完整列表。⌘S 存檔；空白鍵播放／暫停；←/→ ±3 秒；↑/↓ 換段；R 重播本段；N 下一個未確認的存疑處；Enter 編輯目前段；編輯中 Esc 播放／暫停、⌘Enter 完成。

## 測試

```bash
go test ./...
SERMON_PROOF_CORPUS=/path/to/N59_雅各書 go test ./internal/transcript   # 對全部真實檔做 round-trip
```

開發紀錄在 `ai-log/`（給 AI 讀，英文）。

## 發行（維護者）

版本號在 `internal/version/version.go`（三段式 semver）。改版、commit 後：

```bash
go run ./tools/release        # dist/ 產生三個平台壓縮檔與 SHA256SUMS.txt（有未 commit 的改動會拒絕）
go run ./tools/npmpack        # dist/npm/ 產生四個 .tgz（三個平台包 + 啟動器包），並印出 npm publish 指令
```

兩支工具都不會上傳。npm 要先發三個平台包、最後發 `@fhlnet/sermon-proof`；GitHub Release 則自行把 `dist/` 的壓縮檔與 `SHA256SUMS.txt` 附上。
