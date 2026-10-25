// Package filedlg shows the native macOS "choose file" dialog through osascript.
package filedlg

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"strings"
	"time"
)

// ErrCancelled is returned when the user dismisses the dialog.
var ErrCancelled = errors.New("cancelled")

var kinds = map[string]struct{ prompt, types string }{
	"md":    {"選擇逐字稿 (.md)", `{"md"}`},
	"audio": {"選擇音檔", `{"mp3","m4a","wav"}`},
}

// Pick shows the dialog for kind ("md" or "audio") and returns a POSIX path.
func Pick(kind string) (string, error) {
	k, ok := kinds[kind]
	if !ok {
		return "", errors.New("unknown kind " + kind)
	}
	script := `POSIX path of (choose file with prompt "` + k.prompt + `" of type ` + k.types + `)`
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "osascript", "-e", script)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		if strings.Contains(errb.String(), "-128") { // User canceled.
			return "", ErrCancelled
		}
		return "", errors.New(strings.TrimSpace(errb.String()))
	}
	return strings.TrimSpace(out.String()), nil
}
