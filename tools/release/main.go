// Command release builds the distributable archives: one per target holding the
// binary and README.md, plus SHA256SUMS.txt.
//
// Run from the repository root:
//
//	go run ./tools/release                          # all targets into dist/
//	go run ./tools/release -targets windows/amd64
//
// Builds are CGO_ENABLED=0 and -trimpath, and archive mtimes come from the git
// commit, so the same commit gives the same files. Nothing is uploaded.
package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"sermonproof/internal/version"
)

const appName = "sermon-proof"

type target struct{ goos, goarch string }

func (t target) exe() string {
	if t.goos == "windows" {
		return appName + ".exe"
	}
	return appName
}

func (t target) archive(ver string) string {
	ext := "tar.gz"
	if t.goos == "windows" {
		ext = "zip"
	}
	return fmt.Sprintf("%s_%s_%s_%s.%s", appName, ver, t.goos, t.goarch, ext)
}

type file struct {
	name string
	data []byte
	mode int64
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "release:", err)
		os.Exit(1)
	}
}

func run() error {
	out := flag.String("out", "dist", "output directory (created if missing)")
	targetsFlag := flag.String("targets", "windows/amd64,darwin/arm64,linux/amd64", "comma-separated GOOS/GOARCH list")
	allowDirty := flag.Bool("allow-dirty", false, "allow building with uncommitted changes")
	flag.Parse()

	if _, err := os.Stat("go.mod"); err != nil {
		return fmt.Errorf("run from the repository root (no go.mod here)")
	}
	var targets []target
	for _, s := range strings.Split(*targetsFlag, ",") {
		os_, arch, ok := strings.Cut(strings.TrimSpace(s), "/")
		if !ok {
			return fmt.Errorf("bad target %q (want GOOS/GOARCH)", s)
		}
		targets = append(targets, target{os_, arch})
	}
	when, dirty, err := gitInfo()
	if err != nil {
		return err
	}
	if dirty && !*allowDirty {
		return fmt.Errorf("uncommitted changes; commit first or pass -allow-dirty")
	}
	readme, err := os.ReadFile("README.md")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(*out, 0o755); err != nil {
		return err
	}

	ver := version.Version
	tmp, err := os.MkdirTemp("", "sermon-proof-release-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)

	sums := map[string]string{}
	for _, t := range targets {
		bin := filepath.Join(tmp, t.goos+"_"+t.goarch, t.exe())
		cmd := exec.Command("go", "build", "-trimpath", "-ldflags", "-s -w", "-o", bin, ".")
		cmd.Env = append(os.Environ(), "CGO_ENABLED=0", "GOOS="+t.goos, "GOARCH="+t.goarch)
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("build %s/%s: %w", t.goos, t.goarch, err)
		}
		data, err := os.ReadFile(bin)
		if err != nil {
			return err
		}
		top := strings.TrimSuffix(strings.TrimSuffix(t.archive(ver), ".tar.gz"), ".zip")
		files := []file{
			{top + "/" + t.exe(), data, 0o755},
			{top + "/README.md", readme, 0o644},
		}
		var blob []byte
		if t.goos == "windows" {
			blob, err = makeZip(files, when)
		} else {
			blob, err = makeTarGz(files, when)
		}
		if err != nil {
			return err
		}
		name := t.archive(ver)
		if err := os.WriteFile(filepath.Join(*out, name), blob, 0o644); err != nil {
			return err
		}
		h := sha256.Sum256(blob)
		sums[name] = hex.EncodeToString(h[:])
		fmt.Printf("%s  %d bytes\n", name, len(blob))
	}

	var names []string
	for n := range sums {
		names = append(names, n)
	}
	sort.Strings(names)
	var sb strings.Builder
	for _, n := range names {
		fmt.Fprintf(&sb, "%s  %s\n", sums[n], n)
	}
	return os.WriteFile(filepath.Join(*out, "SHA256SUMS.txt"), []byte(sb.String()), 0o644)
}

// gitInfo returns the HEAD commit time and whether the working tree has changes.
func gitInfo() (time.Time, bool, error) {
	b, err := exec.Command("git", "log", "-1", "--format=%ct").Output()
	if err != nil {
		return time.Time{}, false, fmt.Errorf("git log: %w", err)
	}
	sec, err := strconv.ParseInt(strings.TrimSpace(string(b)), 10, 64)
	if err != nil {
		return time.Time{}, false, err
	}
	st, err := exec.Command("git", "status", "--porcelain", "--untracked-files=no").Output()
	if err != nil {
		return time.Time{}, false, fmt.Errorf("git status: %w", err)
	}
	return time.Unix(sec, 0).UTC(), len(bytes.TrimSpace(st)) > 0, nil
}

func makeZip(files []file, when time.Time) ([]byte, error) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, f := range files {
		h := &zip.FileHeader{Name: f.name, Method: zip.Deflate, Modified: when}
		h.SetMode(os.FileMode(f.mode))
		w, err := zw.CreateHeader(h)
		if err != nil {
			return nil, err
		}
		if _, err := w.Write(f.data); err != nil {
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func makeTarGz(files []file, when time.Time) ([]byte, error) {
	var buf bytes.Buffer
	gz, _ := gzip.NewWriterLevel(&buf, gzip.BestCompression)
	tw := tar.NewWriter(gz)
	for _, f := range files {
		h := &tar.Header{Typeflag: tar.TypeReg, Name: f.name, Mode: f.mode, Size: int64(len(f.data)), ModTime: when, Format: tar.FormatPAX}
		if err := tw.WriteHeader(h); err != nil {
			return nil, err
		}
		if _, err := tw.Write(f.data); err != nil {
			return nil, err
		}
	}
	if err := tw.Close(); err != nil {
		return nil, err
	}
	if err := gz.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
