# Cross-platform builds, release archives and npm packages (@fhlnet/sermon-proof)

- Date: 2026-10-27
- Project: sermon-proof
- Type: feature
- Related logs: 1026d_drag_drop_open.md
- Cross-project: model followed github.com/snowray712000/FHL-MCP-Go (`tools/release`, `tools/npmpack`)
- Commit: uncommitted

## Goal / why
User wanted releases for Windows / macOS / Linux and `npm i -g` installation, like FHL-MCP-Go. Org scope on npm: `@fhlnet`.

## Reference project findings
FHL-MCP-Go has no CI and no `.github/`. Releases are produced locally by two stdlib-only Go programs and published by hand: `tools/release` (cross-build, archives, `SHA256SUMS.txt`) and `tools/npmpack` (reads the release archives, verifies them against the sums, writes npm `.tgz` files; esbuild-style layout: one launcher package + one package per platform in `optionalDependencies`). Archive names `<app>_<ver>_<os>_<arch>.{zip|tar.gz}`. Same approach copied here (no CI added; a tag-triggered GitHub Actions workflow is an obvious later step but needs an `NPM_TOKEN` secret, which was not set up).

## Code changes
- `internal/version/version.go`: `const Version = "0.1.0"`; single source for both tools. `main.go` gained `-version` (prints `sermon-proof <ver>`).
- Cross-platform runtime (previously macOS only):
  - `main.go openBrowser`: `open` (darwin), `rundll32 url.dll,FileProtocolHandler` (windows), `xdg-open` (others).
  - `internal/filedlg`: one `Pick(kind)` dispatching on `runtime.GOOS`. darwin: osascript as before (cancel = stderr contains `-128`). windows: `powershell -NoProfile -STA -Command` with `System.Windows.Forms.OpenFileDialog`, output encoding forced to UTF-8, cancel = `exit 2`. Linux: `zenity --file-selection` or `kdialog --getopenfilename` (cancel = exit 1); neither installed → error telling the user to drag the file or pass it on the command line.
  - `internal/locate`: `spotlight()` returns nil unless darwin (folder walk remains the fallback); `uriPath` strips the leading `/` of `file:///C:/x` on Windows.
- `tools/release/main.go` (`go run ./tools/release [-out dist] [-targets windows/amd64,darwin/arm64,linux/amd64] [-allow-dirty]`): refuses a dirty tracked tree; builds with `CGO_ENABLED=0 go build -trimpath -ldflags "-s -w"`; archive `sermon-proof_<ver>_<os>_<arch>.{zip for windows, tar.gz}` containing `<same-name>/sermon-proof[.exe]` and `README.md`; mtimes = HEAD commit time; writes sorted `SHA256SUMS.txt` (`<sha256>  <name>`).
- `tools/npmpack/main.go` (`go run ./tools/npmpack [-dist dist] [-scope fhlnet] [-version X]`): requires three-part semver; verifies each archive's sha256 against `SHA256SUMS.txt`; extracts only the binary; writes `dist/npm/*.tgz`: `@fhlnet/sermon-proof-{win32-x64,darwin-arm64,linux-x64}` (package.json with `os`/`cpu`, `bin/<exe>`) and `@fhlnet/sermon-proof` (launcher `bin/sermon-proof.js`, `optionalDependencies` pinned to the same version, `engines.node >=16`, `publishConfig.access=public`). Launcher resolves `<platform pkg>/bin/<exe>` via `require.resolve`, chmods it, spawns with `stdio: inherit`, forwards exit code/signal. Prints `npm publish` commands (platform packages first, launcher last); never publishes.
- `.gitignore`: `/dist/`. README gained install + maintainer release sections; CLAUDE.md platform line and a release note.

## Decisions / gotchas
- Supported targets are only windows/amd64, darwin/arm64, linux/amd64 (matches the reference; add rows to `platforms` in npmpack and `-targets` in release for others, plus the launcher's `PACKAGES` map).
- No LICENSE file exists in this repo, so package.json has no `license` field and archives carry none. Add one before wide distribution.
- npm only takes formal semver; publish order matters because the launcher's optionalDependencies must already exist on the registry.
- No code signing / notarization.

## Verification
`go vet ./...`, `go test ./...` pass; `GOOS=windows|linux|darwin go build` all succeed; both tools run end to end; `npm i` of the darwin-arm64 tgz plus the launcher tgz into a scratch project and `node_modules/.bin/sermon-proof -version` printed `sermon-proof 0.1.0`. NOT verified: running on real Windows / Linux, the PowerShell / zenity / kdialog dialogs, an actual `npm publish`, and the registry install path.
