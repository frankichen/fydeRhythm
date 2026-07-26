# fydeRhythm 个人词库同步服务生产部署记录

本文记录 `shulufa.555044.xyz` 的正式部署方式，后续升级和故障排查以本文为准。

## 生产环境

- 服务器：`oracle-2`，Ubuntu 24.04 LTS
- 域名：`shulufa.555044.xyz`
- Git 分支：`feature/personal-sync-server`
- Compose 目录：`/opt/fyderhythm/runtime`
- 容器：`fyderhythm-sync`
- 容器监听：`:8080`
- 宿主机监听：`127.0.0.1:18081`
- 数据目录：`/opt/fyderhythm/runtime/data`
- 备份目录：`/opt/fyderhythm/backups`
- 公网地址：`https://shulufa.555044.xyz`

原有 `subconv` 已占用 `127.0.0.1:18080`，不能停止或覆盖，因此本服务使用 `18081`。同步服务没有直接暴露公网端口，公网流量只经过现有 Docker Nginx 的 80/443。

## 安全注意事项

- `API_TOKEN` 只保存在服务器 `/opt/fyderhythm/runtime/.env`，权限必须是 `600`。
- 不要把 `.env`、token、Authorization 请求头写入日志、Issue、聊天记录或 Git 提交。
- 不要执行 `docker compose down`、删除数据目录，或重启现有 `nginx`、`subconv` 等项目来排查本服务。
- 服务容器以 UID `10001` 运行，数据目录必须属于 `10001:10001`。

## 如何安全取得 API_TOKEN

登录服务器后，在当前 shell 中读取 token，但不要打印它：

```bash
ssh root@oracle-2
cd /opt/fyderhythm/runtime
export API_TOKEN="$(awk -F= '/^API_TOKEN=/{print substr($0,index($0,"=")+1)}' .env)"
```

只检查长度，不显示内容：

```bash
printf 'API_TOKEN length: '
printf %s "$API_TOKEN" | wc -c
```

不要使用 `cat .env`、`echo "$API_TOKEN"`，也不要把 token 直接写进 shell 历史或脚本。

## 查看服务状态

```bash
ssh root@oracle-2
cd /opt/fyderhythm/runtime
docker compose ps
docker inspect fyderhythm-sync --format 'status={{.State.Status}} health={{if .State.Health}}{{.State.Health.Status}}{{else}}none{{end}}'
docker logs --tail 100 fyderhythm-sync
ss -lntp | grep -E '127\.0\.0\.1:18081|127\.0\.0\.1:18080'
```

正常结果应包含 `Up (healthy)` 和 `127.0.0.1:18081->8080/tcp`。

## 后续升级：本地编译，服务器只运行二进制镜像

服务器不执行 `go build`，也不使用服务器源码编译 Docker 镜像。

### 1. 本地检查和编译

```bash
git fetch origin
git switch feature/personal-sync-server
git pull --ff-only origin feature/personal-sync-server
cd sync-server
gofmt -w $(rg --files -g '*.go')
go vet ./...
go test ./...
CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /tmp/fyderhythm-sync ./cmd/server
```

### 2. 本地构建运行时镜像

`Dockerfile.runtime` 只复制已经编译好的二进制，不会在镜像构建阶段编译 Go：

```bash
cd sync-server
cp /tmp/fyderhythm-sync ./fyderhythm-sync
docker build --network=host --file Dockerfile.runtime --tag fyderhythm-sync:runtime .
rm -f ./fyderhythm-sync
```

### 3. 上传镜像并重启同步服务

下面命令只传输本地生成的运行时镜像，不包含 token：

```bash
docker save fyderhythm-sync:runtime | gzip | ssh root@oracle-2 'gzip -d | docker load'
ssh root@oracle-2 'docker tag fyderhythm-sync:runtime fyderhythm-sync:runtime && cd /opt/fyderhythm/runtime && docker compose up -d --no-build && docker compose ps'
```

升级前后确认没有覆盖 `.env` 或数据目录：

```bash
ssh root@oracle-2 'stat -c "%a %U:%G %n" /opt/fyderhythm/runtime/.env /opt/fyderhythm/runtime/data'
```

## API 冒烟测试

```bash
ssh root@oracle-2 'bash -s' <<'EOF'
set -eu
API_TOKEN="$(awk -F= '/^API_TOKEN=/{print substr($0,index($0,"=")+1)}' /opt/fyderhythm/runtime/.env)"
BASE='https://shulufa.555044.xyz'
curl -fsS "$BASE/healthz"
curl -fsS -H "Authorization: Bearer $API_TOKEN" "$BASE/api/v1/stats"
curl -fsS -X POST -H "Authorization: Bearer $API_TOKEN" -H 'Content-Type: application/json' -H 'X-Device-ID: deploy-check' -d '{"phrase":"部署检查词","shortcut":"deploy-check"}' "$BASE/api/v1/lexicon"
curl -fsS -H "Authorization: Bearer $API_TOKEN" "$BASE/api/v1/lexicon?q=部署检查词"
EOF
```

确认响应禁止缓存：

```bash
API_TOKEN="$(awk -F= '/^API_TOKEN=/{print substr($0,index($0,"=")+1)}' /opt/fyderhythm/runtime/.env)"
curl -sS -D - -o /dev/null -H "Authorization: Bearer $API_TOKEN" https://shulufa.555044.xyz/api/v1/stats | grep -Ei '^(cache-control|cf-cache-status):'
```

预期包含 `Cache-Control: no-store` 和 `CF-Cache-Status: DYNAMIC`。

## 数据备份与恢复

备份由 systemd timer 执行，每天 UTC 03:17，保留 14 天：

```bash
systemctl status fyderhythm-sync-backup.timer --no-pager
systemctl list-timers fyderhythm-sync-backup.timer
find /opt/fyderhythm/backups -maxdepth 1 -type f -name 'fyderhythm-sync-*.db' -ls
```

手动执行一次备份：

```bash
systemctl start fyderhythm-sync-backup.service
```

恢复前保留当前数据库副本：

```bash
cd /opt/fyderhythm/runtime
docker compose stop
cp data/fyderhythm-sync.db data/fyderhythm-sync.db.before-restore
cp /opt/fyderhythm/backups/fyderhythm-sync-YYYYMMDDTHHMMSSZ.db data/fyderhythm-sync.db
chown 10001:10001 data/fyderhythm-sync.db
docker compose start
```

不要删除旧备份或数据库，除非已经确认恢复结果并有另外的副本。

## Nginx 和 Cloudflare

现有 Nginx 配置文件：`/home/ubuntu/dly/nginx/conf.d/shulufa.conf`。

```bash
docker exec nginx nginx -t
docker exec nginx nginx -s reload
```

Cloudflare 控制台必须保持：

1. `shulufa.555044.xyz` DNS 指向该 Oracle 服务器，并开启橙色云代理。
2. SSL/TLS 加密模式为 `Full (strict)`。
3. API 路径（至少 `/api/*`，最好整个该域名）设置为不缓存。
4. 不要对带 `Authorization` 请求头的 API 响应启用缓存。

源站证书：

```text
/etc/letsencrypt/live/555044.xyz/fullchain.pem
/etc/letsencrypt/live/555044.xyz/privkey.pem
```

该证书包含 `*.555044.xyz`，覆盖 `shulufa.555044.xyz`。

## 回滚原则

- 不停止、删除或重建现有 `nginx`、`subconv`、`vaultwarden`、`gogs` 等容器。
- 只操作 Compose 项目 `/opt/fyderhythm/runtime` 和容器 `fyderhythm-sync`。
- 回滚时保留当前数据目录和 `.env`，切换到已验证的旧 `fyderhythm-sync:runtime` 镜像后执行：

```bash
cd /opt/fyderhythm/runtime
docker compose up -d --no-build
docker compose ps
```
