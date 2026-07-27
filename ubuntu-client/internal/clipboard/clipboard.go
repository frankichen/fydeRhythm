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

// ReadText reads plain text from the desktop clipboard. It prefers the
// native command for the current display server, then tries the other common
// Linux clipboard backend. Clipboard contents are never logged.
func ReadText(ctx context.Context) (string, error) {
	commands := make([]readerCommand, 0, 2)
	if strings.TrimSpace(os.Getenv("WAYLAND_DISPLAY")) != "" {
		commands = append(commands, readerCommand{name: "wl-paste", args: []string{"--no-newline", "--type", "text"}})
	}
	if strings.TrimSpace(os.Getenv("DISPLAY")) != "" {
		commands = append(commands, readerCommand{name: "xclip", args: []string{"-selection", "clipboard", "-o"}})
	}
	if len(commands) == 0 {
		commands = append(commands,
			readerCommand{name: "wl-paste", args: []string{"--no-newline", "--type", "text"}},
			readerCommand{name: "xclip", args: []string{"-selection", "clipboard", "-o"}},
		)
	} else if commands[0].name == "wl-paste" {
		commands = append(commands, readerCommand{name: "xclip", args: []string{"-selection", "clipboard", "-o"}})
	} else {
		commands = append(commands, readerCommand{name: "wl-paste", args: []string{"--no-newline", "--type", "text"}})
	}

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
	text := strings.TrimSpace(strings.ReplaceAll(string(output), "\x00", ""))
	if text == "" {
		return "", errors.New("剪贴板没有文本；请先选中需要纠错的文字")
	}
	return text, nil
}
