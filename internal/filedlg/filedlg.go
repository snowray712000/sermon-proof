// Package filedlg shows the platform's native "choose file" dialog: osascript on
// macOS, PowerShell on Windows, zenity or kdialog on Linux.
package filedlg

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// ErrCancelled is returned when the user dismisses the dialog.
var ErrCancelled = errors.New("cancelled")

type kindInfo struct {
	prompt string
	exts   []string
}

var kinds = map[string]kindInfo{
	"md":    {"選擇逐字稿 (.md / .vtt)", []string{"md", "vtt"}},
	"audio": {"選擇音檔", []string{"mp3", "m4a", "wav"}},
}

// Pick shows the dialog for kind ("md" = a .md or .vtt transcript, or "audio") and returns the chosen path.
func Pick(kind string) (string, error) {
	k, ok := kinds[kind]
	if !ok {
		return "", errors.New("unknown kind " + kind)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	switch runtime.GOOS {
	case "darwin":
		return pickDarwin(ctx, k)
	case "windows":
		return pickWindows(ctx, k)
	default:
		return pickLinux(ctx, k)
	}
}

// run executes cmd and returns trimmed stdout; cancelled maps a failed run to ErrCancelled.
func run(cmd *exec.Cmd, cancelled func(exitCode int, stderr string) bool) (string, error) {
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		code := -1
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			code = ee.ExitCode()
		}
		if cancelled(code, errb.String()) {
			return "", ErrCancelled
		}
		if msg := strings.TrimSpace(errb.String()); msg != "" {
			return "", errors.New(msg)
		}
		return "", err
	}
	return strings.TrimSpace(out.String()), nil
}

func pickDarwin(ctx context.Context, k kindInfo) (string, error) {
	types := `{"` + strings.Join(k.exts, `","`) + `"}`
	script := `POSIX path of (choose file with prompt "` + k.prompt + `" of type ` + types + `)`
	return run(exec.CommandContext(ctx, "osascript", "-e", script), func(_ int, stderr string) bool {
		return strings.Contains(stderr, "-128") // User canceled.
	})
}

func pickWindows(ctx context.Context, k kindInfo) (string, error) {
	var pats []string
	for _, e := range k.exts {
		pats = append(pats, "*."+e)
	}
	script := `Add-Type -AssemblyName System.Windows.Forms;` +
		`$d = New-Object System.Windows.Forms.OpenFileDialog;` +
		`$d.Title = '` + k.prompt + `';` +
		`$d.Filter = '` + strings.Join(k.exts, `/`) + ` (` + strings.Join(pats, `;`) + `)|` + strings.Join(pats, `;`) + `';` +
		`if ($d.ShowDialog() -eq 'OK') { [Console]::OutputEncoding = [Text.Encoding]::UTF8; $d.FileName } else { exit 2 }`
	cmd := exec.CommandContext(ctx, "powershell", "-NoProfile", "-STA", "-Command", script)
	return run(cmd, func(code int, _ string) bool { return code == 2 })
}

func pickLinux(ctx context.Context, k kindInfo) (string, error) {
	var pats []string
	for _, e := range k.exts {
		pats = append(pats, "*."+e)
	}
	if p, err := exec.LookPath("zenity"); err == nil {
		cmd := exec.CommandContext(ctx, p, "--file-selection", "--title="+k.prompt,
			"--file-filter="+k.prompt+" | "+strings.Join(pats, " "))
		return run(cmd, func(code int, _ string) bool { return code == 1 })
	}
	if p, err := exec.LookPath("kdialog"); err == nil {
		cmd := exec.CommandContext(ctx, p, "--title", k.prompt, "--getopenfilename", ".", strings.Join(pats, " "))
		return run(cmd, func(code int, _ string) bool { return code == 1 })
	}
	return "", errors.New("找不到 zenity 或 kdialog，無法顯示選檔視窗；請改用拖曳檔案到頁面，或在命令列帶檔名開啟")
}
