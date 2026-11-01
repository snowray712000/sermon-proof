# 新專案 bootstrap prompt（來自 sermon-proof 的作法）

用法：開新專案的 Claude Code session，把下面「--- 開始 ---」到「--- 結束 ---」整段貼進去。
參考實作在本機 `/Users/Shared/coding/信望愛/人工校正語音辨識/`（GitHub：`snowray712000/sermon-proof`）。新專案若在同一台機器，AI 可以直接讀那個資料夾；否則請先 `git clone` 到某處再把路徑告訴它。

--- 開始 ---

我要在這個新專案採用我另一個專案 **sermon-proof** 的開發作法。參考實作在 `/Users/Shared/coding/信望愛/人工校正語音辨識/`（**唯讀參考，不要修改它**）。請先讀懂它，再把下列慣例搬到目前專案，**照著做、不要自己重新發明**。

## 0. 先問我、不要猜

動手前，用 AskUserQuestion 一次問完下列參數（有預設值的附在括號）：

- `APP_NAME`：執行檔／指令名稱（kebab-case，例如 `sermon-proof`）
- 一句話專案定位、主要語言（預設 Go 1.x，只用標準庫）、是否有前端（預設原生 HTML/JS/CSS，`go:embed`）
- 要不要發行到 npm？npm scope（預設 `@fhlnet`）、GitHub repo URL
- 支援平台（預設 windows/amd64、darwin/arm64、linux/amd64）
- 有沒有「使用者原始資料」要設為唯讀（有的話路徑為何）
- 有沒有兄弟專案需要在 ai-log 以 `Cross-project:` 互相指向

若不是 Go 專案：第 1、2、3、5 節照用；第 4 節（發行與 npm）只保留概念（可重現建置、SHA256SUMS、optionalDependencies 結構、不自動上傳），實作改成該語言對應的作法，並先跟我確認。

## 1. CLAUDE.md（專案根目錄，繁體中文）

仿 `CLAUDE.md` 的章節結構：

1. **專案定位**：做什麼、技術棧、平台、與兄弟專案／原始資料的關係（原始資料唯讀，工具輸出不寫進去；測試樣本複製到 `testdata/`）。
2. **不可違反的原則**：把這個專案的硬規則寫成條列，每條一句話＋為什麼。通用的幾條照搬：
   - 只改使用者要求的檔案；修改前先看目標檔，不覆蓋未備份內容。
   - 不引入第三方套件（Go 與前端皆然），要加先問。不用 npm 管前端依賴。
   - 會動到使用者檔案的功能：先備份（只做一次、之後絕不覆蓋）、先寫暫存檔再 rename、寫入前比對 hash 偵測外部改動。
   - 改動核心邏輯後必須跑 `go test ./...`。
3. **技術備忘**：建置／測試／執行指令、目錄說明、容易踩雷的細節（格式 regex 只在某處使用之類的跨檔耦合要寫下來）。
4. **Git**：`git init`；**只有我要求時才 commit / push**；不要 stage 編譯出的執行檔、`.DS_Store`、`.claude/`；commit 訊息用繁體中文簡短描述。
5. **ai-log**：見第 2 節，必須把「開始前讀 index、完成後寫 log」的規定寫進 CLAUDE.md。

## 2. ai-log（給未來 AI 的開發紀錄）

1. 複製 `ai-log/README.md` 並改成本專案（專案名稱、`Cross-project` 段落）。規則不要改：英文撰寫；`ai-log/<YYYY>/s<Q>/<MMDD><seq>_<snake_case_title>.md`（s1=1–3 月、s2=4–6、s3=7–9、s4=10–12；日期取系統當天，不要猜；seq 為 a…z、aa、ab…，依長度再字母序）；append-only，舊 log 只能修錯字／加 `Superseded by:`／加 `## Addendum`；不寫入金鑰密碼；目標是「只靠 log 就能大致重現程式」（行為、資料格式、API、演算法、決策與踩雷），不是列改了哪些檔。
2. 建立 `ai-log/index.md`：開頭 2–3 行專案說明＋指向 README；每季一節（新季在前、新 entry 在前），表格欄位 `Log | Type | Summary | Areas / keywords | Files touched`，一列一行。
3. 範本章節固定：Goal / why、Starting state、What changed、Design and decisions、Key files、Contracts and algorithms、Gotchas、Verification、Open issues / next steps。**Verification 要誠實**：明寫哪些「沒有驗證」（例如沒在真實 Windows 跑過、沒真的 `npm publish`）。
4. 工作流程：開始非瑣碎任務前先讀 `ai-log/index.md`，只讀相關 log；完成有意義的工作單元後寫一份 log＋更新 index（同一個 commit，如果我要求 commit）。第一份 log 用 `Type: baseline` 記錄本次 bootstrap 做了什麼。
5. 參考 `ai-log/2026/s4/1027a_cross_platform_release_and_npm.md` 的詳細程度與語氣。

## 3. 專案骨架與衛生

- `README.md`（繁體中文）：安裝（npm / 下載發行檔，附 SHA256 說明與「無簽章，Gatekeeper／SmartScreen 可能警告」）、建置與執行、做什麼不做什麼、測試、維護者發行流程。
- `.gitignore`：`.DS_Store`、`.claude/`、`/<APP_NAME>`、`/<APP_NAME>.exe`、`*.exe`、`*.test`、`/dist/`。（sermon-proof 目前漏了 `*.exe`，導致根目錄的 exe 出現在 untracked，別重蹈。）
- **加 `LICENSE`**（先問我要哪種授權）。sermon-proof 沒有，npm 包因此沒有 `license` 欄位。
- `internal/version/version.go`：`const Version = "0.1.0"`，三段式 semver（npm 只收正式版號），是 release／npmpack／`-version` 旗標的唯一來源。
- 目錄慣例：`main.go`、`internal/<模組>/`、`web/`（embed）、`tools/release`、`tools/npmpack`、`testdata/`。
- 本機 web 工具的安全習慣（若適用）：只綁 `127.0.0.1`、隨機 port、API 用隨機 token 防護、提供 `-port`／`-no-open`／`-version`。
- 跨平台：開瀏覽器 `open`／`rundll32 url.dll,FileProtocolHandler`／`xdg-open`；按 OS 分派的功能集中在單一套件；`CGO_ENABLED=0`。

## 4. 發行與 npm（Go 專案；不上傳，發佈由我手動）

讀 `tools/release/main.go`、`tools/npmpack/main.go` 與 log `1027a`，複製到新專案後只改下列**寫死的地方**（grep `sermon-proof`、`sermonproof`、`snowray712000`）：

- 兩支工具 import 的 module 路徑（`sermonproof/internal/version` → 新 module）。
- `tools/release`：`appName`、預設 `-targets`、壓縮檔內附的檔案。
- `tools/npmpack`：`repoURL`、`mainPkg`、`platforms` 表、`-scope` 預設、套件描述與兩份 README 文字、`launcher` 裡的 `PACKAGES` 與執行檔名。

必須保留的行為（這些是踩過／設計過的重點）：

- **命名**：壓縮檔 `<app>_<ver>_<os>_<arch>.{zip(windows)|tar.gz}`，內含單一頂層資料夾（執行檔＋README.md）；`SHA256SUMS.txt` 格式 `<sha256>  <name>`、依名稱排序。
- **可重現**：`CGO_ENABLED=0 go build -trimpath -ldflags "-s -w"`；壓縮檔 mtime 取 HEAD commit 時間；tgz 內容排序固定。
- **髒樹拒絕**：有未 commit 的追蹤檔變更就拒絕建置（`-allow-dirty` 才放行）。流程是：改版號 → commit → `go run ./tools/release` → `go run ./tools/npmpack`。
- **npm 結構（esbuild 式 optionalDependencies）**：主包 `@scope/<app>` 只有 `bin/<app>.js` 啟動器；每個平台一包 `@scope/<app>-<nodeOS>-<nodeCPU>`（`win32-x64`、`darwin-arm64`、`linux-x64`），`package.json` 帶 `os`／`cpu`、`bin/<exe>`；主包 `optionalDependencies` 鎖同一版本、`engines.node >=16`、`publishConfig.access=public`。
- **npmpack 不重新編譯**：從 release 壓縮檔取出執行檔，先對照 `SHA256SUMS.txt` 驗證，讓 GitHub Release 與 npm 的執行檔逐位元組相同。
- **啟動器**：`require.resolve` 找平台包的執行檔、非 Windows 時 chmod 755、`spawn` 用 `stdio: inherit`、轉送 exit code／signal、找不到時給出明確錯誤（提示 `--omit=optional`）。
- **發佈順序**：先發三個平台包，**最後**才發主包（主包的 optionalDependencies 必須已存在於 registry）。工具只印出 `npm publish` 指令，**絕不自己上傳**。
- 要我先有 npm 帳號／組織（scope）並 `npm login`；首次發佈需 `--access public`（`publishConfig` 已涵蓋）。

建置完成後請實際驗證：三個平台 `GOOS=… go build` 都過；兩支工具跑通；把 darwin-arm64 的 tgz 與主包 tgz 裝進暫存專案，執行 `node_modules/.bin/<app> -version` 印出版本。**驗證不到的（真實 Windows／Linux 執行、真的 npm publish）要在 log 與回報裡明說沒驗證。**

## 5. 工作方式

- 每個非瑣碎任務：先讀 `ai-log/index.md` → 實作 → `go vet ./... && go test ./...` → 寫 ai-log＋更新 index → 回報（含沒驗證的部分）。
- 不要主動 commit／push；我說要才做。
- 要新增任何第三方依賴、改動原始資料資料夾、或做不可逆的事，先問我。

完成後：列出你建立／修改的檔案、你問我的參數與採用的值、以及還缺什麼（例如 CI、程式碼簽章、LICENSE 選擇）。

--- 結束 ---

## 附註：你可能忘了、或目前還沒做的項目（給你自己看，不在 prompt 裡）

- **LICENSE**：sermon-proof 沒有（見 log 1027a）。已在 prompt 第 3 節要求新專案補上。
- **.gitignore 漏洞**：目前 `sermon-proof.exe`、`sermonproof.exe` 沒被忽略（只有 `/sermon-proof`）。新專案 prompt 已避開；這個專案要不要順手補 `*.exe`，由你決定。
- **CI**：兩個專案都沒有 GitHub Actions；tag 觸發發行需要 `NPM_TOKEN` secret，目前刻意手動發佈。
- **程式碼簽章／Apple 公證**：沒做，README 已說明會有系統警告。
- **memory／全域 `~/.claude/CLAUDE.md`**：你的 commit 繁中、不自動 commit 等偏好目前寫在每個專案的 CLAUDE.md；若想新專案不用重貼，可把「Git 規則＋ai-log 流程」抽成使用者層級設定（需要的話我可以幫你做）。
