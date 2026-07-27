package app

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/frankichen/fydeRhythm/ubuntu-client/internal/aiservice"
	"github.com/frankichen/fydeRhythm/ubuntu-client/internal/config"
	"github.com/frankichen/fydeRhythm/ubuntu-client/internal/deepseek"
)

type AIConfigOptions struct {
	BaseURL          string
	Model            string
	ProxyURL         string
	APIKey           string
	ReadKeyFromStdin bool
	Enable           bool
	Disable          bool
	NonInteractive   bool
	TimeoutSeconds   int
	MaxInputChars    int
}

func (a *App) ConfigureAI(options AIConfigOptions) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if options.Enable && options.Disable {
		return errors.New("不能同时启用和禁用 DeepSeek")
	}
	if strings.TrimSpace(options.BaseURL) != "" {
		cfg.AI.BaseURL = strings.TrimSpace(options.BaseURL)
	}
	if strings.TrimSpace(options.Model) != "" {
		cfg.AI.Model = strings.TrimSpace(options.Model)
	}
	if options.ProxyURL != "" {
		cfg.AI.ProxyURL = strings.TrimSpace(options.ProxyURL)
	}
	if options.TimeoutSeconds > 0 {
		cfg.AI.TimeoutSeconds = options.TimeoutSeconds
	}
	if options.MaxInputChars > 0 {
		cfg.AI.MaxInputChars = options.MaxInputChars
	}

	key := strings.TrimSpace(options.APIKey)
	if key == "" {
		key = strings.TrimSpace(os.Getenv("DEEPSEEK_API_KEY"))
	}
	if key == "" && options.ReadKeyFromStdin {
		data, readErr := io.ReadAll(io.LimitReader(os.Stdin, 16<<10))
		if readErr != nil {
			return fmt.Errorf("读取 DeepSeek Key: %w", readErr)
		}
		key = strings.TrimSpace(string(data))
	}
	if key == "" && !options.NonInteractive && !options.Disable {
		var readErr error
		key, readErr = readSecret(a.Out, "请输入 DeepSeek API Key：")
		if readErr != nil {
			return readErr
		}
	}
	if key != "" {
		cfg.AI.APIKey = key
	}
	if options.Disable {
		cfg.AI.Enabled = false
	} else if options.Enable || key != "" {
		cfg.AI.Enabled = true
	}
	if err := config.Save(cfg); err != nil {
		return err
	}
	fmt.Fprintf(a.Out, "DeepSeek：%s\n", enabledText(cfg.AI.Enabled))
	fmt.Fprintf(a.Out, "模型：%s\n", cfg.AI.Model)
	fmt.Fprintf(a.Out, "地址：%s\n", cfg.AI.BaseURL)
	fmt.Fprintf(a.Out, "代理：%s\n", emptyAs(cfg.AI.ProxyURL, "跟随系统环境变量/直连"))
	fmt.Fprintf(a.Out, "Key：%s\n", config.MaskSecret(cfg.AI.APIKey))
	fmt.Fprintln(a.Out, "配置已保存到权限 0600 的本地配置文件，不会同步到个人词库服务器。")
	return nil
}

func (a *App) AITest(ctx context.Context) error {
	client, cfg, err := loadAIClient()
	if err != nil {
		return err
	}
	result, err := client.Test(ctx)
	if err != nil {
		return err
	}
	fmt.Fprintf(a.Out, "DeepSeek 连接正常：%s（%s）\n", result, cfg.AI.Model)
	return nil
}

func (a *App) AICorrect(ctx context.Context, text string) error {
	client, _, err := loadAIClient()
	if err != nil {
		return err
	}
	text, err = textOrStdin(text)
	if err != nil {
		return err
	}
	result, err := client.Correct(ctx, text)
	if err != nil {
		return err
	}
	fmt.Fprintln(a.Out, result)
	return nil
}

func (a *App) AIPredict(ctx context.Context, text string) error {
	client, _, err := loadAIClient()
	if err != nil {
		return err
	}
	text, err = textOrStdin(text)
	if err != nil {
		return err
	}
	result, err := client.Predict(ctx, text)
	if err != nil {
		return err
	}
	fmt.Fprintln(a.Out, result)
	return nil
}

func (a *App) AIServe(ctx context.Context) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if _, err := deepseek.New(cfg.AI); err != nil {
		return err
	}
	server := aiservice.New(config.RuntimeSocketPath())
	fmt.Fprintf(a.Out, "fydeRhythm AI 服务正在监听 %s\n", server.SocketPath)
	return server.Serve(ctx)
}

func (a *App) AIStatus() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	fmt.Fprintf(a.Out, "启用：%s\n", enabledText(cfg.AI.Enabled))
	fmt.Fprintf(a.Out, "模型：%s\n", cfg.AI.Model)
	fmt.Fprintf(a.Out, "地址：%s\n", cfg.AI.BaseURL)
	fmt.Fprintf(a.Out, "代理：%s\n", emptyAs(cfg.AI.ProxyURL, "跟随系统环境变量/直连"))
	fmt.Fprintf(a.Out, "Key：%s\n", config.MaskSecret(cfg.AI.APIKey))
	fmt.Fprintf(a.Out, "Socket：%s\n", config.RuntimeSocketPath())
	return nil
}

func loadAIClient() (*deepseek.Client, config.Config, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, config.Config{}, err
	}
	client, err := deepseek.New(cfg.AI)
	if err != nil {
		return nil, config.Config{}, err
	}
	return client, cfg, nil
}

func textOrStdin(value string) (string, error) {
	if strings.TrimSpace(value) != "" {
		return value, nil
	}
	data, err := io.ReadAll(io.LimitReader(os.Stdin, 128<<10))
	if err != nil {
		return "", err
	}
	value = strings.TrimSpace(string(data))
	if value == "" {
		return "", errors.New("请用 --text 提供文本，或通过标准输入传入文本")
	}
	return value, nil
}

func readSecret(out io.Writer, prompt string) (string, error) {
	fmt.Fprint(out, prompt)
	terminal := false
	if stat, err := os.Stdin.Stat(); err == nil {
		terminal = stat.Mode()&os.ModeCharDevice != 0
	}
	if terminal {
		_ = exec.Command("stty", "-echo").Run()
		defer func() {
			_ = exec.Command("stty", "echo").Run()
			fmt.Fprintln(out)
		}()
	}
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	return strings.TrimSpace(line), nil
}

func enabledText(value bool) string {
	if value {
		return "已启用"
	}
	return "未启用"
}

func emptyAs(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
