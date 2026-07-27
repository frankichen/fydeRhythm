# fydeRhythm Ubuntu 同步客户端

面向 Ubuntu + Fcitx5 + Rime 的本地优先个人词库同步客户端。

## 当前预览版能力

- 从 `https://shulufa.555044.xyz` 下载个人词库；
- 本地 JSON 状态和增量游标，断网不影响输入；
- 生成 `fyderhythm_personal.dict.yaml`；
- 自动给 `wubi86_jidian_pinyin_smart` 增加独立个人词典 translator；
- 每五分钟通过 systemd user timer 增量同步；
- `download-only`、`full`、`manual` 三种模式；
- `full` 模式下通过命令行新增、删除个人词；
- Token 仅保存在权限 `0600` 的用户配置文件中。

第一版暂不解析 Rime 的二进制用户词频库，因此 Ubuntu 本机自动学习到的候选次数还不会上传；Rime 自身的本地学习不受影响。服务器个人词库的下载、手动词条上传及跨设备同步已经可用。

## 推荐：公司电脑使用仅下载模式

```bash
fyderhythm-sync init --mode download-only
fyderhythm-sync test
fyderhythm-sync sync --force
systemctl --user enable --now fyderhythm-sync.timer
```

首次初始化会提示输入 API Token。Token 保存在：

```text
~/.config/fyderhythm/config.json
```

## 安装 `.deb`

```bash
sudo apt install ./fyderhythm-sync_0.1.0_amd64.deb
```

然后安装五笔方案：

```bash
RIME_DIR="$HOME/.local/share/fcitx5/rime"
TMP_DIR="$(mktemp -d)"
git clone --depth 1 https://github.com/frankichen/rime-wubi86-jidian.git "$TMP_DIR/rime-wubi86-jidian"
mkdir -p "$RIME_DIR"
rsync -a --exclude='.git' "$TMP_DIR/rime-wubi86-jidian/" "$RIME_DIR/"
rm -rf "$TMP_DIR"
im-config -n fcitx5
```

注销并重新登录，在 `fcitx5-configtool` 中添加 `Rime`。

## 常用命令

```bash
fyderhythm-sync doctor
fyderhythm-sync status
fyderhythm-sync test
fyderhythm-sync sync --force
fyderhythm-sync list
```

个人电脑需要上传手动词库时：

```bash
fyderhythm-sync init --mode full
fyderhythm-sync add --phrase "示例专有词" --shortcut "slzc" --weight 300 --category "常用"
fyderhythm-sync sync --force
```

删除时先执行 `list` 获取 ID：

```bash
fyderhythm-sync remove --id lex-xxxxxxxx
fyderhythm-sync sync --force
```

## 本地文件

```text
~/.config/fyderhythm/config.json
~/.config/fyderhythm/state.json
~/.local/share/fcitx5/rime/fyderhythm_personal.dict.yaml
~/.local/share/fcitx5/rime/wubi86_jidian_pinyin_smart.custom.yaml
```

客户端不会上传完整输入内容。第一版上传的内容仅限你通过 `add/remove` 明确管理的个人词条。
