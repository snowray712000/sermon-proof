// Command npmpack turns an already-built release (dist/*.zip|*.tar.gz plus
// SHA256SUMS.txt, produced by tools/release) into npm packages: one launcher
// package and one package per platform carrying the binary (the
// "optionalDependencies" layout used by esbuild).
//
//	go run ./tools/npmpack                 # dist/ -> dist/npm/*.tgz for internal/version.Version
//	go run ./tools/npmpack -scope fhlnet   # @fhlnet/sermon-proof, @fhlnet/sermon-proof-win32-x64, ...
//
// Binaries come from the release archives (after checking SHA256SUMS.txt), so the
// npm packages and the GitHub release carry byte-identical executables. Nothing is
// published: the tool writes .tgz files and prints the commands to run by hand.
package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"sermonproof/internal/version"
)

var semverRe = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$`)

const (
	repoURL = "https://github.com/snowray712000/sermon-proof"
	mainPkg = "sermon-proof"
)

type platform struct {
	goos, goarch string // as in the release archive name
	nodeOS       string // process.platform / package.json "os"
	nodeCPU      string // process.arch / package.json "cpu"
	exe          string
}

var platforms = []platform{
	{"windows", "amd64", "win32", "x64", "sermon-proof.exe"},
	{"darwin", "arm64", "darwin", "arm64", "sermon-proof"},
	{"linux", "amd64", "linux", "x64", "sermon-proof"},
}

func (p platform) key() string { return p.nodeOS + "-" + p.nodeCPU }

type file struct {
	name string
	data []byte
	mode int64
}

func main() {
	dist := flag.String("dist", "dist", "directory holding the release archives and SHA256SUMS.txt")
	out := flag.String("out", "", "output directory (default <dist>/npm)")
	scope := flag.String("scope", "fhlnet", "npm scope (organization) without @")
	ver := flag.String("version", version.Version, "version to package; the release archives for it must exist in -dist")
	flag.Parse()
	if *out == "" {
		*out = filepath.Join(*dist, "npm")
	}
	names, err := run(*dist, *out, *scope, *ver)
	if err != nil {
		fmt.Fprintln(os.Stderr, "npmpack:", err)
		os.Exit(1)
	}
	fmt.Printf("\n已產生 %d 個 npm 包於 %s（尚未發佈）。發佈指令（先 npm login；先發平台包，最後發主包）：\n", len(names), *out)
	for _, n := range names {
		fmt.Printf("  npm publish %s\n", filepath.ToSlash(filepath.Join(*out, n)))
	}
}

// run builds every package and returns the .tgz file names in publish order (main package last).
func run(dist, out, scope, ver string) ([]string, error) {
	if !semverRe.MatchString(ver) {
		return nil, fmt.Errorf("version %q is not three-part semver such as 1.2.3", ver)
	}
	sums, err := readSums(filepath.Join(dist, "SHA256SUMS.txt"))
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(out, 0o755); err != nil {
		return nil, err
	}
	var names []string
	var when time.Time // from the last archive; keeps the output reproducible
	for _, p := range platforms {
		ext := "tar.gz"
		if p.goos == "windows" {
			ext = "zip"
		}
		archive := fmt.Sprintf("%s_%s_%s_%s.%s", mainPkg, ver, p.goos, p.goarch, ext)
		bin, w, err := extract(filepath.Join(dist, archive), sums, archive, p.exe)
		if err != nil {
			return nil, err
		}
		when = w
		pkg := fmt.Sprintf("@%s/%s-%s", scope, mainPkg, p.key())
		pj, err := platformJSON(scope, pkg, ver, p)
		if err != nil {
			return nil, err
		}
		files := []file{
			{"package.json", pj, 0o644},
			{"README.md", []byte(platformReadme(scope, p)), 0o644},
			{"bin/" + p.exe, bin, 0o755},
		}
		name := tgzName(pkg, ver)
		if err := writeTgz(filepath.Join(out, name), files, when); err != nil {
			return nil, err
		}
		names = append(names, name)
		fmt.Printf("%s  %s\n", pkg, name)
	}
	pkg := fmt.Sprintf("@%s/%s", scope, mainPkg)
	pj, err := mainJSON(scope, ver)
	if err != nil {
		return nil, err
	}
	files := []file{
		{"package.json", pj, 0o644},
		{"README.md", []byte(mainReadme(scope)), 0o644},
		{"bin/sermon-proof.js", []byte(strings.ReplaceAll(launcher, "@SCOPE@", "@"+scope)), 0o755},
	}
	name := tgzName(pkg, ver)
	if err := writeTgz(filepath.Join(out, name), files, when); err != nil {
		return nil, err
	}
	fmt.Printf("%s  %s\n", pkg, name)
	return append(names, name), nil
}

func tgzName(pkg, ver string) string {
	return strings.ReplaceAll(strings.TrimPrefix(pkg, "@"), "/", "-") + "-" + ver + ".tgz"
}

// readSums parses "<sha256>  <name>" lines.
func readSums(p string) (map[string]string, error) {
	b, err := os.ReadFile(p)
	if err != nil {
		return nil, fmt.Errorf("read SHA256SUMS.txt (run tools/release first): %w", err)
	}
	m := map[string]string{}
	for _, ln := range strings.Split(string(b), "\n") {
		f := strings.Fields(ln)
		if len(f) == 2 {
			m[f[1]] = f[0]
		}
	}
	return m, nil
}

// extract verifies the archive against SHA256SUMS.txt and returns the named
// top-level file and the archive's modification time.
func extract(archivePath string, sums map[string]string, archive, want string) ([]byte, time.Time, error) {
	raw, err := os.ReadFile(archivePath)
	if err != nil {
		return nil, time.Time{}, err
	}
	h := sha256.Sum256(raw)
	if exp, ok := sums[archive]; !ok {
		return nil, time.Time{}, fmt.Errorf("%s is not listed in SHA256SUMS.txt", archive)
	} else if exp != hex.EncodeToString(h[:]) {
		return nil, time.Time{}, fmt.Errorf("%s does not match SHA256SUMS.txt (rebuild or re-download it)", archive)
	}
	// A file directly inside the archive's single top folder.
	isWanted := func(name string) bool {
		return strings.Count(strings.Trim(name, "/"), "/") == 1 && path.Base(name) == want
	}
	var data []byte
	var when time.Time
	if strings.HasSuffix(archive, ".zip") {
		zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
		if err != nil {
			return nil, time.Time{}, err
		}
		for _, f := range zr.File {
			if f.FileInfo().IsDir() || !isWanted(f.Name) {
				continue
			}
			rc, err := f.Open()
			if err != nil {
				return nil, time.Time{}, err
			}
			data, err = io.ReadAll(rc)
			rc.Close()
			if err != nil {
				return nil, time.Time{}, err
			}
			when = f.Modified
		}
	} else {
		gz, err := gzip.NewReader(bytes.NewReader(raw))
		if err != nil {
			return nil, time.Time{}, err
		}
		tr := tar.NewReader(gz)
		for {
			hd, err := tr.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				return nil, time.Time{}, err
			}
			if hd.Typeflag != tar.TypeReg || !isWanted(hd.Name) {
				continue
			}
			if data, err = io.ReadAll(tr); err != nil {
				return nil, time.Time{}, err
			}
			when = hd.ModTime
		}
	}
	if len(data) == 0 {
		return nil, time.Time{}, fmt.Errorf("%s: %s not found", archive, want)
	}
	return data, when.UTC(), nil
}

type pkgJSON struct {
	Name                 string            `json:"name"`
	Version              string            `json:"version"`
	Description          string            `json:"description"`
	Homepage             string            `json:"homepage"`
	Repository           map[string]string `json:"repository"`
	Bin                  map[string]string `json:"bin,omitempty"`
	OS                   []string          `json:"os,omitempty"`
	CPU                  []string          `json:"cpu,omitempty"`
	Engines              map[string]string `json:"engines,omitempty"`
	OptionalDependencies map[string]string `json:"optionalDependencies,omitempty"`
	PublishConfig        map[string]string `json:"publishConfig"`
	Files                []string          `json:"files"`
}

func marshal(p pkgJSON) ([]byte, error) {
	p.Homepage = repoURL
	p.Repository = map[string]string{"type": "git", "url": "git+" + repoURL + ".git"}
	p.PublishConfig = map[string]string{"access": "public"}
	b, err := json.MarshalIndent(p, "", "  ")
	return append(b, '\n'), err
}

func platformJSON(scope, pkg, ver string, p platform) ([]byte, error) {
	return marshal(pkgJSON{
		Name: pkg, Version: ver,
		Description: fmt.Sprintf("sermon-proof 講道逐字稿校對工具的 %s 執行檔；請安裝 @%s/%s 而非直接安裝本套件", p.key(), scope, mainPkg),
		OS:          []string{p.nodeOS}, CPU: []string{p.nodeCPU},
		Files: []string{"bin/"},
	})
}

func mainJSON(scope, ver string) ([]byte, error) {
	opt := map[string]string{}
	for _, p := range platforms {
		opt[fmt.Sprintf("@%s/%s-%s", scope, mainPkg, p.key())] = ver
	}
	return marshal(pkgJSON{
		Name: fmt.Sprintf("@%s/%s", scope, mainPkg), Version: ver,
		Description:          "sermon-proof：邊聽音檔邊人工校對講道逐字稿（.md）的本機小工具",
		Bin:                  map[string]string{"sermon-proof": "bin/sermon-proof.js"},
		Engines:              map[string]string{"node": ">=16"},
		OptionalDependencies: opt,
		Files:                []string{"bin/"},
	})
}

func mainReadme(scope string) string {
	return fmt.Sprintf("# @%[1]s/sermon-proof\n\n"+
		"邊聽音檔邊人工校對講道逐字稿（.md）並直接寫回的本機小工具，只綁 127.0.0.1，用瀏覽器操作。安裝後得到 `sermon-proof` 指令：\n\n"+
		"```sh\nnpm i -g @%[1]s/sermon-proof\nsermon-proof 檔名.md        # 自動配對同名音檔\nsermon-proof               # 不帶參數，於頁面按「開啟」選檔\n```\n\n"+
		"也可以不安裝：`npx @%[1]s/sermon-proof 檔名.md`。\n\n"+
		"本套件只有啟動器，執行檔在對應你系統的 `@%[1]s/sermon-proof-<系統>` 套件（Windows x64、macOS arm64、Linux x64），npm 會自動挑選。\n"+
		"安裝時若加了 `--omit=optional`／`--no-optional` 就不會裝到執行檔。\n\n"+
		"完整說明與原始碼：%[2]s\n", scope, repoURL)
}

func platformReadme(scope string, p platform) string {
	return fmt.Sprintf("# @%[1]s/sermon-proof-%[2]s\n\n這是 `@%[1]s/sermon-proof` 在 %[2]s 上的執行檔，不需要直接安裝；請改裝 `@%[1]s/sermon-proof`。\n\n原始碼：%[3]s\n", scope, p.key(), repoURL)
}

// launcher finds the binary of the matching platform package and runs it with the same arguments.
const launcher = `#!/usr/bin/env node
'use strict';
const { spawn } = require('child_process');
const fs = require('fs');

const PACKAGES = {
  'win32-x64': '@SCOPE@/sermon-proof-win32-x64',
  'darwin-arm64': '@SCOPE@/sermon-proof-darwin-arm64',
  'linux-x64': '@SCOPE@/sermon-proof-linux-x64',
};

function fail(msg) {
  process.stderr.write(msg + '\n');
  process.exit(1);
}

const key = process.platform + '-' + process.arch;
const pkg = PACKAGES[key];
if (!pkg) {
  fail('不支援的系統：' + key + '。支援 Windows x64、macOS arm64、Linux x64；其他系統請到 GitHub Release 下載或自行編譯。');
}
const exe = process.platform === 'win32' ? 'sermon-proof.exe' : 'sermon-proof';
let bin;
try {
  bin = require.resolve(pkg + '/bin/' + exe);
} catch (e) {
  fail('找不到執行檔套件 ' + pkg + '。可能安裝時用了 --omit=optional／--no-optional，或安裝不完整；請重新執行 npm i -g @SCOPE@/sermon-proof。');
}
if (process.platform !== 'win32') {
  try { fs.chmodSync(bin, 0o755); } catch (e) { /* already executable, or not ours to change */ }
}

const child = spawn(bin, process.argv.slice(2), { stdio: 'inherit' });
// The terminal sends Ctrl+C to the whole group, so the child shuts itself down; just wait for it.
process.on('SIGINT', () => {});
process.on('SIGTERM', () => child.kill('SIGTERM'));
child.on('error', (e) => fail('無法執行 ' + bin + '：' + e.message));
child.on('exit', (code, signal) => {
  if (signal) {
    process.removeAllListeners(signal);
    process.kill(process.pid, signal);
  } else {
    process.exit(code === null ? 1 : code);
  }
});
`

// writeTgz writes an npm-style tarball (everything under "package/"), reproducibly.
func writeTgz(p string, files []file, when time.Time) error {
	sort.Slice(files, func(i, j int) bool { return files[i].name < files[j].name })
	var buf bytes.Buffer
	gz, _ := gzip.NewWriterLevel(&buf, gzip.BestCompression)
	tw := tar.NewWriter(gz)
	dirs := map[string]bool{}
	for _, f := range files {
		for d := path.Dir(f.name); d != "."; d = path.Dir(d) {
			dirs[d] = true
		}
	}
	var dn []string
	for d := range dirs {
		dn = append(dn, d)
	}
	sort.Strings(dn)
	for _, d := range dn {
		if err := tw.WriteHeader(&tar.Header{Typeflag: tar.TypeDir, Name: "package/" + d + "/", Mode: 0o755, ModTime: when, Format: tar.FormatPAX}); err != nil {
			return err
		}
	}
	for _, f := range files {
		h := &tar.Header{Typeflag: tar.TypeReg, Name: "package/" + f.name, Mode: f.mode, Size: int64(len(f.data)), ModTime: when, Format: tar.FormatPAX}
		if err := tw.WriteHeader(h); err != nil {
			return err
		}
		if _, err := tw.Write(f.data); err != nil {
			return err
		}
	}
	if err := tw.Close(); err != nil {
		return err
	}
	if err := gz.Close(); err != nil {
		return err
	}
	return os.WriteFile(p, buf.Bytes(), 0o644)
}
