package aiservice

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/frankichen/fydeRhythm/ubuntu-client/internal/clipboard"
	"github.com/frankichen/fydeRhythm/ubuntu-client/internal/config"
	"github.com/frankichen/fydeRhythm/ubuntu-client/internal/deepseek"
)

const (
	protocolVersion = "FYDEAI/1"
	maxRequestBytes = 64 << 10
	maxWorkers      = 2
)

type Server struct {
	SocketPath string
	sem        chan struct{}
	mu         sync.Mutex
	listener   net.Listener
}

func New(socketPath string) *Server {
	if strings.TrimSpace(socketPath) == "" {
		socketPath = config.RuntimeSocketPath()
	}
	return &Server{SocketPath: socketPath, sem: make(chan struct{}, maxWorkers)}
}

func (s *Server) Serve(ctx context.Context) error {
	if err := os.MkdirAll(filepath.Dir(s.SocketPath), 0o700); err != nil {
		return fmt.Errorf("创建 AI socket 目录: %w", err)
	}
	if err := removeStaleSocket(s.SocketPath); err != nil {
		return err
	}
	listener, err := net.Listen("unix", s.SocketPath)
	if err != nil {
		return fmt.Errorf("监听 AI socket: %w", err)
	}
	if err := os.Chmod(s.SocketPath, 0o600); err != nil {
		listener.Close()
		return fmt.Errorf("设置 AI socket 权限: %w", err)
	}
	s.mu.Lock()
	s.listener = listener
	s.mu.Unlock()
	defer func() {
		listener.Close()
		_ = os.Remove(s.SocketPath)
	}()

	go func() {
		<-ctx.Done()
		s.mu.Lock()
		if s.listener != nil {
			_ = s.listener.Close()
		}
		s.mu.Unlock()
	}()

	for {
		conn, err := listener.Accept()
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, net.ErrClosed) {
				return nil
			}
			return fmt.Errorf("接受 AI socket 连接: %w", err)
		}
		select {
		case s.sem <- struct{}{}:
			go func() {
				defer func() { <-s.sem }()
				s.handle(ctx, conn)
			}()
		default:
			_ = writeResponse(conn, false, "AI 正忙，请稍后重试")
			_ = conn.Close()
		}
	}
}

func (s *Server) handle(parent context.Context, conn net.Conn) {
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(90 * time.Second))
	action, text, err := readRequest(conn)
	if err != nil {
		_ = writeResponse(conn, false, err.Error())
		return
	}

	// Capture the clipboard before forwarding Ctrl+C. This local action does
	// not require a valid DeepSeek client.
	if action == "clipboard_read" {
		ctx, cancel := context.WithTimeout(parent, 3*time.Second)
		defer cancel()
		result, readErr := clipboard.Snapshot(ctx)
		if readErr != nil {
			_ = writeResponse(conn, false, readErr.Error())
			return
		}
		_ = writeResponse(conn, true, result)
		return
	}

	cfg, err := config.Load()
	if err != nil {
		_ = writeResponse(conn, false, err.Error())
		return
	}
	client, err := deepseek.New(cfg.AI)
	if err != nil {
		_ = writeResponse(conn, false, err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(parent, time.Duration(cfg.AI.TimeoutSeconds+5)*time.Second)
	defer cancel()

	var result string
	switch action {
	case "ping":
		result, err = client.Test(ctx)
	case "correct":
		result, err = client.Correct(ctx, text)
	case "predict":
		result, err = client.Predict(ctx, text)
	case "correct_clipboard":
		var selected string
		selected, err = clipboard.WaitForChange(ctx, text, 2*time.Second)
		if err == nil {
			result, err = client.Correct(ctx, selected)
		}
	default:
		err = fmt.Errorf("不支持的 AI 操作 %q", action)
	}
	if err != nil {
		_ = writeResponse(conn, false, err.Error())
		return
	}
	_ = writeResponse(conn, true, result)
}

func readRequest(r io.Reader) (string, string, error) {
	reader := bufio.NewReaderSize(r, 4096)
	header, err := reader.ReadString('\n')
	if err != nil {
		return "", "", fmt.Errorf("读取 AI 请求头: %w", err)
	}
	if len(header) > 4096 {
		return "", "", errors.New("AI 请求头过长")
	}
	parts := strings.Fields(strings.TrimSpace(header))
	if len(parts) != 3 || parts[0] != protocolVersion {
		return "", "", errors.New("AI 请求协议不正确")
	}
	length, err := strconv.Atoi(parts[2])
	if err != nil || length < 0 || length > maxRequestBytes {
		return "", "", errors.New("AI 请求长度不正确")
	}
	body := make([]byte, length)
	if _, err := io.ReadFull(reader, body); err != nil {
		return "", "", fmt.Errorf("读取 AI 请求正文: %w", err)
	}
	return parts[1], string(body), nil
}

func writeResponse(w io.Writer, ok bool, body string) error {
	status := "ERR"
	if ok {
		status = "OK"
	}
	data := []byte(body)
	if _, err := fmt.Fprintf(w, "%s %d\n", status, len(data)); err != nil {
		return err
	}
	_, err := w.Write(data)
	return err
}

func removeStaleSocket(path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSocket == 0 {
		return fmt.Errorf("拒绝覆盖非 socket 文件 %s", path)
	}
	conn, dialErr := net.DialTimeout("unix", path, 300*time.Millisecond)
	if dialErr == nil {
		conn.Close()
		return fmt.Errorf("已有 AI 服务正在监听 %s", path)
	}
	return os.Remove(path)
}
