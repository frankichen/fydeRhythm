package clipboard

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

const maxClipboardBytes = 128 << 10

type readerCommand struct {
	name string
	args []string
}

// ReadText reads non-empty plain text from the desktop clipboard. Clipboard
// contents are never logged.
func ReadText(ctx context.Context) (string, error) {
	text, err := Snapshot(ctx)
	if err != nil {
		return "", err
	}
	if text == "" {
		return "", errors.New("剪贴板没有文本；请先选中需要纠错的文字")
	}
	return text, nil
}

// Snapshot reads the current plain-text clipboard value and permits an empty
// clipboard. It is used before forwarding Ctrl+C so stale clipboard text can
// be distinguished from the newly copied selection.
func Snapshot(ctx context.Context) (string, error) {
	commands := readerCommands()
	var failures []string
	for _, candidate := range commands {
		text, err := run(ctx, candidate)
		if err == nil {
			return text, nil
		}
		failures = append(failures, candidate.name+": "+err.Error())
	}
	return "", fmt.Errorf("无法读取系统剪贴板（%s）", strings.Join(failures, "；"))
}

// WaitForChange waits until Ctrl+C has replaced the previous clipboard value.
// Returning an error instead of reusing the old value is intentional: stale
// clipboard text must never be sent to DeepSeek as if it were the selection.
func WaitForChange(parent context.Context, previous string, timeout time.Duration) (string, error) {
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()

	ticker := time.NewTicker(80 * time.Millisecond)
	defer ticker.Stop()
	for {
		text, err := Snapshot(ctx)
		if err == nil && text != "" && text != previous {
			return text, nil
		}
		select {
		case <-ctx.Done():
			return "", errors.New("没有取得当前选中的文字；请保持选中状态后重试")
		case <-ticker.C:
		}
	}
}

func readerCommands() []readerCommand {
	commands := make([]readerCommand, 0, 2)
	if strings.TrimSpace(os.Getenv("WAYLAND_DISPLAY")) != "" {
		commands = append(commands, readerCommand{name: "wl-paste", args: []string{"--no-newline", "--type", "text"}})
	}
	if strings.TrimSpace(os.Getenv("DISPLAY")) != "" {
		commands = append(commands, readerCommand{name: "xclip", args: []string{"-selection", "clipboard", "-o"}})
	}
	if len(commands) == 0 {
		return []readerCommand{
			{name: "wl-paste", args: []string{"--no-newline", "--type", "text"}},
			{name: "xclip", args: []string{"-selection", "clipboard", "-o"}},
		}
	}
	if commands[0].name == "wl-paste" {
		commands = append(commands, readerCommand{name: "xclip", args: []string{"-selection", "clipboard", "-o"}})
	} else {
		commands = append(commands, readerCommand{name: "wl-paste", args: []string{"--no-newline", "--type", "text"}})
	}
	return commands
}

func run(parent context.Context, candidate readerCommand) (string, error) {
	if _, err := exec.LookPath(candidate.name); err != nil {
		return "", errors.New("命令未安装")
	}
	ctx, cancel := context.WithTimeout(parent, 3*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, candidate.name, candidate.args...).Output()
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return "", errors.New("读取超时")
		}
		return "", errors.New("读取失败")
	}
	if len(output) > maxClipboardBytes {
		return "", errors.New("内容过长")
	}
	return strings.TrimSpace(strings.ReplaceAll(string(output), "\x00", "")), nil
}
