package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/frankichen/fydeRhythm/ubuntu-client/internal/app"
	"github.com/frankichen/fydeRhythm/ubuntu-client/internal/config"
	"github.com/frankichen/fydeRhythm/ubuntu-client/internal/model"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	a := app.New()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	var err error

	switch os.Args[1] {
	case "version", "--version", "-v":
		fmt.Println(model.Version)
		return
	case "init":
		fs := flag.NewFlagSet("init", flag.ExitOnError)
		server := fs.String("server", "", "同步服务器地址；首次默认 https://shulufa.555044.xyz")
		token := fs.String("token", "", "同步 API Token；建议使用 FYDERHYTHM_API_TOKEN 环境变量")
		mode := fs.String("mode", "", "full|download-only|manual；首次默认 download-only")
		deviceName := fs.String("device-name", "", "设备名称")
		schema := fs.String("schema", "wubi86_jidian_pinyin_smart", "Rime schema id")
		rimeDir := fs.String("rime-dir", "", "Rime 用户目录")
		nonInteractive := fs.Bool("non-interactive", false, "不交互读取 Token")
		_ = fs.Parse(os.Args[2:])
		err = a.Init(*server, *token, *mode, *deviceName, *schema, *rimeDir, *nonInteractive)
	case "test":
		err = a.Test(ctx)
	case "sync":
		fs := flag.NewFlagSet("sync", flag.ExitOnError)
		force := fs.Bool("force", false, "manual 模式也执行")
		_ = fs.Parse(os.Args[2:])
		err = a.Sync(ctx, *force)
	case "status":
		err = a.Status()
	case "doctor":
		err = a.Doctor(ctx)
	case "list":
		err = a.List()
	case "render":
		err = a.Render()
	case "deploy":
		err = a.Deploy()
	case "add":
		fs := flag.NewFlagSet("add", flag.ExitOnError)
		phrase := fs.String("phrase", "", "词语")
		code := fs.String("code", "", "完整编码")
		shortcut := fs.String("shortcut", "", "快捷编码")
		weight := fs.Int("weight", 100, "权重")
		category := fs.String("category", "", "分类")
		notes := fs.String("notes", "", "备注")
		_ = fs.Parse(os.Args[2:])
		err = a.Add(*phrase, *code, *shortcut, *category, *notes, *weight)
	case "remove":
		fs := flag.NewFlagSet("remove", flag.ExitOnError)
		id := fs.String("id", "", "词条 ID")
		_ = fs.Parse(os.Args[2:])
		err = a.Remove(*id)
	case "ai-config":
		fs := flag.NewFlagSet("ai-config", flag.ExitOnError)
		baseURL := fs.String("base-url", "", "DeepSeek API 地址")
		model := fs.String("model", "", "DeepSeek 模型，默认 deepseek-v4-flash")
		proxy := fs.String("proxy", "", "仅供 DeepSeek 使用的 HTTP 代理，例如 http://127.0.0.1:10808")
		clearProxy := fs.Bool("clear-proxy", false, "清除 DeepSeek 专用代理")
		key := fs.String("key", "", "DeepSeek Key；不建议使用，可能进入 shell 历史")
		keyStdin := fs.Bool("key-stdin", false, "从标准输入读取 DeepSeek Key")
		enable := fs.Bool("enable", false, "启用 DeepSeek")
		disable := fs.Bool("disable", false, "禁用 DeepSeek")
		nonInteractive := fs.Bool("non-interactive", false, "不显示交互输入")
		timeout := fs.Int("timeout", 0, "请求超时秒数")
		maxChars := fs.Int("max-chars", 0, "单次最多发送字符数")
		_ = fs.Parse(os.Args[2:])
		proxyValue := *proxy
		if *clearProxy {
			proxyValue = " "
		}
		err = a.ConfigureAI(app.AIConfigOptions{
			BaseURL:          *baseURL,
			Model:            *model,
			ProxyURL:         proxyValue,
			APIKey:           *key,
			ReadKeyFromStdin: *keyStdin,
			Enable:           *enable,
			Disable:          *disable,
			NonInteractive:   *nonInteractive,
			TimeoutSeconds:   *timeout,
			MaxInputChars:    *maxChars,
		})
	case "ai-test":
		err = a.AITest(ctx)
	case "ai-status":
		err = a.AIStatus()
	case "ai-correct":
		fs := flag.NewFlagSet("ai-correct", flag.ExitOnError)
		text := fs.String("text", "", "要纠错的文本；为空时读取标准输入")
		_ = fs.Parse(os.Args[2:])
		err = a.AICorrect(ctx, *text)
	case "ai-predict":
		fs := flag.NewFlagSet("ai-predict", flag.ExitOnError)
		text := fs.String("text", "", "续写上下文；为空时读取标准输入")
		_ = fs.Parse(os.Args[2:])
		err = a.AIPredict(ctx, *text)
	case "ai-serve":
		serveCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		err = a.AIServe(serveCtx)
	default:
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "错误：", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Printf(`fyderhythm-sync - Ubuntu/Fcitx5 个人词库同步与 DeepSeek AI 客户端

同步命令：
  init         初始化同步服务器、Token 和设备模式
  test         测试同步服务器连接及鉴权
  sync         执行一次词库同步
  status       查看同步状态
  doctor       检查 Fcitx5、Rime、方案和服务器
  list         列出本地个人词库
  add/remove   管理个人词（仅 full 模式）
  render       重新生成 Rime 词典
  deploy       重新部署 Rime

DeepSeek 命令：
  ai-config    配置 DeepSeek Key、模型和代理
  ai-test      测试 DeepSeek
  ai-status    查看脱敏后的 AI 配置
  ai-correct   在终端测试文本纠错
  ai-predict   在终端测试下一句续写
  ai-serve     启动 Fcitx5 插件使用的本地 AI 服务

默认 DeepSeek：%s / %s
Fcitx5 快捷键：Alt+R 纠错，Alt+Enter 续写
`, config.DefaultDeepSeekBaseURL, config.DefaultDeepSeekModel)
}
