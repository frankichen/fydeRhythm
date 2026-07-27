package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/frankichen/fydeRhythm/ubuntu-client/internal/app"
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
		token := fs.String("token", "", "API Token；建议使用 FYDERHYTHM_API_TOKEN 环境变量")
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
	fmt.Print(`fyderhythm-sync - Ubuntu/Fcitx5 个人词库同步客户端

命令：
  init      初始化服务器、Token 和设备模式
  test      测试服务器连接及鉴权
  sync      执行一次同步
  status    查看同步状态
  doctor    检查 Fcitx5、Rime、方案和服务器
  list      列出本地个人词库
  add       添加个人词（仅 full 模式）
  remove    删除个人词（仅 full 模式）
  render    重新生成 Rime 词典
  deploy    重新部署 Rime
  version   显示版本
`)
}
