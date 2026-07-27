# fydeRhythm Ubuntu：Fcitx5/Rime + DeepSeek AI

Ubuntu 0.2 版把三部分组合在一起：

1. Fcitx5 + Rime 的五笔86极点与拼音混输；
2. 自建服务器上的个人词库增量同步；
3. 真正调用 DeepSeek API 的手动纠错和下一句续写。

## DeepSeek 功能

安装后，在 Ubuntu 应用菜单打开 **fydeRhythm AI 设置**，填写：

- DeepSeek API Key；
- 模型，默认 `deepseek-v4-flash`；
- 可选 HTTP 代理，例如公司电脑使用 `http://127.0.0.1:10808`。

Key 仅写入当前用户的：

```text
~/.config/fyderhythm/config.json
```

文件权限为 `0600`，不会上传到 `shulufa.555044.xyz`，也不会进入个人词库同步数据。

快捷键：

- `Alt+R`：纠错选中文字；没有选区时，纠错光标前最近一句；
- `Alt+Enter`：根据光标前上下文生成下一小段续写；
- AI 候选出现后按 `Enter`、`Space` 或 `1` 接受，按 `Esc` 取消。

AI 不会持续监听或上传每次按键。只有你主动按快捷键时，插件才把选中文字或光标前有限长度的上下文直接发送给 DeepSeek。密码和敏感输入框会被禁用。

## 升级安装

```bash
sudo apt install ./fyderhythm-sync_0.2.0_amd64.deb
systemctl --user daemon-reload
fcitx5-remote -r
```

打开应用菜单中的 **fydeRhythm AI 设置**。也可以使用命令行：

```bash
read -rsp "DeepSeek API Key: " DEEPSEEK_API_KEY; echo
export DEEPSEEK_API_KEY
fyderhythm-sync ai-config \
  --enable \
  --model deepseek-v4-flash \
  --proxy http://127.0.0.1:10808 \
  --non-interactive
unset DEEPSEEK_API_KEY

fyderhythm-sync ai-test
systemctl --user enable --now fyderhythm-ai.service
fcitx5-remote -r
```

不需要代理时使用：

```bash
fyderhythm-sync ai-config --clear-proxy --enable
```

## 终端独立测试

```bash
fyderhythm-sync ai-status
fyderhythm-sync ai-test
echo '这个句子有错务。' | fyderhythm-sync ai-correct
echo '我们已经完成了服务器部署，下一步' | fyderhythm-sync ai-predict
```

## 个人词库同步

公司电脑建议保持仅下载：

```bash
fyderhythm-sync init --mode download-only
fyderhythm-sync test
fyderhythm-sync sync --force
systemctl --user enable --now fyderhythm-sync.timer
```

同步域名需要直连时，可给终端及 systemd 设置：

```text
NO_PROXY=localhost,127.0.0.1,::1,shulufa.555044.xyz
```

DeepSeek 可以独立使用配置中的代理，不受同步服务器 `NO_PROXY` 规则影响。

## 运行结构

```text
Fcitx5/Rime
   ├─ 本地正常输入、词频学习
   ├─ fyderhythm-sync：个人词库同步
   └─ libfyderhythmai.so
          │ Unix socket（0600）
          ▼
      fyderhythm-ai.service
          │ HTTPS，可选 127.0.0.1:10808 代理
          ▼
      DeepSeek API
```

普通输入不依赖 DeepSeek。AI 服务、网络或 Key 出问题时，五笔和拼音仍然照常使用。
