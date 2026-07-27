package app

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/frankichen/fydeRhythm/ubuntu-client/internal/config"
	"github.com/frankichen/fydeRhythm/ubuntu-client/internal/model"
	"github.com/frankichen/fydeRhythm/ubuntu-client/internal/remote"
	"github.com/frankichen/fydeRhythm/ubuntu-client/internal/rime"
	"github.com/frankichen/fydeRhythm/ubuntu-client/internal/state"
)

type App struct {
	Out *os.File
	Err *os.File
}

func New() *App { return &App{Out: os.Stdout, Err: os.Stderr} }

func (a *App) Init(serverURL, token, mode, deviceName, schemaID, rimeDir string, nonInteractive bool) error {
	cfg, err := config.Load()
	existing := err == nil
	if !existing {
		cfg, err = config.Default()
		if err != nil {
			return err
		}
	}
	if strings.TrimSpace(serverURL) != "" { cfg.ServerURL = strings.TrimSpace(serverURL) }
	if strings.TrimSpace(mode) != "" { cfg.Mode = strings.TrimSpace(mode) }
	if strings.TrimSpace(deviceName) != "" { cfg.DeviceName = strings.TrimSpace(deviceName) }
	if strings.TrimSpace(schemaID) != "" { cfg.SchemaID = strings.TrimSpace(schemaID) }
	if strings.TrimSpace(rimeDir) != "" { cfg.RimeUserDir = expandHome(strings.TrimSpace(rimeDir)) }
	if token == "" { token = os.Getenv("FYDERHYTHM_API_TOKEN") }
	if token == "" && existing { token = cfg.APIToken }
	if token == "" && !nonInteractive {
		fmt.Fprint(a.Out, "请输入服务器 API Token（输入不会隐藏，请确认周围无人）：")
		line, readErr := bufio.NewReader(os.Stdin).ReadString('\n')
		if readErr != nil && !errors.Is(readErr, os.ErrClosed) { return readErr }
		token = strings.TrimSpace(line)
	}
	cfg.APIToken = strings.TrimSpace(token)
	if err := config.Save(cfg); err != nil { return err }
	if !existing {
		if err := state.Save(state.Empty()); err != nil { return err }
	}
	fmt.Fprintf(a.Out, "配置已保存：%s\n", cfg.ServerURL)
	fmt.Fprintf(a.Out, "设备：%s（%s）\n", cfg.DeviceName, cfg.DeviceID)
	fmt.Fprintf(a.Out, "同步模式：%s\n", cfg.Mode)
	return nil
}

func (a *App) Test(ctx context.Context) error {
	cfg, err := config.Load()
	if err != nil { return err }
	client := remote.New(cfg)
	if err := client.Health(ctx); err != nil { return fmt.Errorf("健康检查失败: %w", err) }
	if _, err := client.Pull(ctx, 0, 1); err != nil { return fmt.Errorf("鉴权检查失败: %w", err) }
	fmt.Fprintln(a.Out, "服务器连接和鉴权正常")
	return nil
}

func (a *App) Sync(ctx context.Context, force bool) error {
	lock, err := state.AcquireLock()
	if err != nil { return err }
	defer lock.Close()
	cfg, err := config.Load()
	if err != nil { return err }
	if cfg.Mode == config.ModeManual && !force {
		fmt.Fprintln(a.Out, "当前为 manual 模式，定时任务已跳过；手工执行请使用 sync --force")
		return nil
	}
	st, err := state.Load()
	if err != nil { return err }
	client := remote.New(cfg)
	if !st.Initialized {
		snapshot, snapshotErr := client.Snapshot(ctx)
		if snapshotErr != nil { state.MarkError(&st, snapshotErr); _ = state.Save(st); return snapshotErr }
		applySnapshot(&st, snapshot)
	}
	if cfg.Mode == config.ModeFull && len(st.Pending) > 0 {
		if err := pushPending(ctx, client, cfg, &st); err != nil { state.MarkError(&st, err); _ = state.Save(st); return err }
	}
	if err := pullAll(ctx, client, &st); err != nil { state.MarkError(&st, err); _ = state.Save(st); return err }
	count, err := rime.Render(cfg, st.Lexicon)
	if err != nil { state.MarkError(&st, err); _ = state.Save(st); return err }
	deployErr := rime.Deploy(cfg)
	state.MarkError(&st, deployErr)
	if saveErr := state.Save(st); saveErr != nil { return saveErr }
	fmt.Fprintf(a.Out, "同步完成：游标 %d，个人词条编码 %d，待上传 %d\n", st.Cursor, count, len(st.Pending))
	if deployErr != nil { fmt.Fprintf(a.Err, "提示：%v\n", deployErr) }
	return nil
}

func applySnapshot(st *state.State, snapshot model.Snapshot) {
	st.Lexicon = make(map[string]model.LexiconEntry)
	for _, entry := range snapshot.Lexicon {
		if entry.ID == "" || entry.Deleted { continue }
		st.Lexicon[entry.ID] = entry
	}
	st.Cursor = snapshot.Cursor
	st.Initialized = true
}

func pullAll(ctx context.Context, client *remote.Client, st *state.State) error {
	for page := 0; page < 100; page++ {
		result, err := client.Pull(ctx, st.Cursor, 500)
		if err != nil { return err }
		for _, change := range result.Changes { applyRemoteChange(st, change) }
		if result.Cursor > st.Cursor { st.Cursor = result.Cursor }
		if !result.HasMore { return nil }
	}
	return errors.New("同步分页超过安全上限")
}

func applyRemoteChange(st *state.State, change model.LogChange) {
	if change.Entity != "lexicon" { return }
	var entry model.LexiconEntry
	if err := json.Unmarshal(change.Payload, &entry); err != nil || entry.ID == "" { return }
	if change.Operation == "delete" || entry.Deleted { delete(st.Lexicon, entry.ID); return }
	st.Lexicon[entry.ID] = entry
}

func pushPending(ctx context.Context, client *remote.Client, cfg config.Config, st *state.State) error {
	for len(st.Pending) > 0 {
		batchSize := len(st.Pending)
		if batchSize > 500 { batchSize = 500 }
		batch := append([]model.Change(nil), st.Pending[:batchSize]...)
		result, err := client.Push(ctx, model.PushRequest{DeviceID: cfg.DeviceID, Device: &model.Device{DeviceID: cfg.DeviceID, Name: cfg.DeviceName, Platform: "linux-fcitx5", AppVersion: model.Version}, Changes: batch})
		if err != nil { return err }
		rejected := make(map[string]string)
		for _, item := range result.Rejected { rejected[item.ChangeID] = item.Reason }
		for _, change := range batch {
			if reason, ok := rejected[change.ChangeID]; ok { st.Rejected = append(st.Rejected, state.Rejected{ChangeID: change.ChangeID, Reason: reason, CreatedAt: time.Now().UnixMilli()}) }
		}
		if len(st.Rejected) > 100 { st.Rejected = st.Rejected[len(st.Rejected)-100:] }
		st.Pending = st.Pending[batchSize:]
	}
	return nil
}

func (a *App) Add(phrase, code, shortcut, category, notes string, weight int) error {
	cfg, err := config.Load()
	if err != nil { return err }
	if cfg.Mode != config.ModeFull { return errors.New("当前设备不是 full 模式；公司电脑默认仅下载，不允许上传新词") }
	phrase = strings.TrimSpace(phrase); code = strings.ToLower(strings.TrimSpace(code)); shortcut = strings.ToLower(strings.TrimSpace(shortcut))
	if phrase == "" || (code == "" && shortcut == "") { return errors.New("phrase 必填，code 和 shortcut 至少填写一个") }
	if strings.ContainsAny(code+shortcut, "\t\r\n ") { return errors.New("编码不能包含空格或换行") }
	if weight <= 0 { weight = 100 }
	id, err := config.NewEntryID(); if err != nil { return err }
	changeID, err := config.NewChangeID(); if err != nil { return err }
	now := time.Now().UnixMilli()
	entry := model.LexiconEntry{ID: id, Phrase: phrase, Code: code, Shortcut: shortcut, Weight: weight, Category: strings.TrimSpace(category), Notes: strings.TrimSpace(notes), ClientUpdatedAt: now, SourceDeviceID: cfg.DeviceID}
	payload, _ := json.Marshal(entry)
	st, err := state.Load(); if err != nil { return err }
	st.Lexicon[id] = entry
	st.Pending = append(st.Pending, model.Change{ChangeID: changeID, Entity: "lexicon", Operation: "upsert", ClientUpdatedAt: now, Payload: payload})
	if err := state.Save(st); err != nil { return err }
	if _, err := rime.Render(cfg, st.Lexicon); err != nil { return err }
	fmt.Fprintf(a.Out, "已添加：%s（ID %s），下次同步上传\n", phrase, id)
	return nil
}

func (a *App) Remove(id string) error {
	cfg, err := config.Load(); if err != nil { return err }
	if cfg.Mode != config.ModeFull { return errors.New("当前设备不是 full 模式，不能删除服务器词条") }
	st, err := state.Load(); if err != nil { return err }
	entry, ok := st.Lexicon[strings.TrimSpace(id)]; if !ok { return errors.New("未找到该词条 ID") }
	changeID, err := config.NewChangeID(); if err != nil { return err }
	now := time.Now().UnixMilli(); payload, _ := json.Marshal(model.LexiconEntry{ID: entry.ID})
	delete(st.Lexicon, entry.ID)
	st.Pending = append(st.Pending, model.Change{ChangeID: changeID, Entity: "lexicon", Operation: "delete", ClientUpdatedAt: now, Payload: payload})
	if err := state.Save(st); err != nil { return err }
	_, err = rime.Render(cfg, st.Lexicon)
	return err
}

func (a *App) List() error {
	st, err := state.Load(); if err != nil { return err }
	entries := make([]model.LexiconEntry, 0, len(st.Lexicon))
	for _, entry := range st.Lexicon { entries = append(entries, entry) }
	sort.Slice(entries, func(i, j int) bool { if entries[i].Category == entries[j].Category { return entries[i].Phrase < entries[j].Phrase }; return entries[i].Category < entries[j].Category })
	for _, entry := range entries { fmt.Fprintf(a.Out, "%s\t%s\t%s\t%d\t%s\t%s\n", entry.ID, entry.Phrase, firstNonEmpty(entry.Shortcut, entry.Code), entry.Weight, entry.Category, entry.Notes) }
	return nil
}

func (a *App) Status() error {
	cfg, err := config.Load(); if err != nil { return err }
	st, err := state.Load(); if err != nil { return err }
	configPath, statePath, _, _ := config.Paths()
	fmt.Fprintf(a.Out, "版本：%s\n服务器：%s\n设备：%s (%s)\n模式：%s\nSchema：%s\nRime 目录：%s\n配置文件：%s\n状态文件：%s\n已初始化：%t\n服务器游标：%d\n本地词条：%d\n待上传：%d\n拒绝记录：%d\n", model.Version, cfg.ServerURL, cfg.DeviceName, cfg.DeviceID, cfg.Mode, cfg.SchemaID, cfg.RimeUserDir, configPath, statePath, st.Initialized, st.Cursor, len(st.Lexicon), len(st.Pending), len(st.Rejected))
	if st.LastSyncAt > 0 { fmt.Fprintf(a.Out, "上次同步：%s\n", time.UnixMilli(st.LastSyncAt).Format(time.RFC3339)) }
	if st.LastError != "" { fmt.Fprintf(a.Out, "上次提示：%s\n", st.LastError) }
	return nil
}

func (a *App) Doctor(ctx context.Context) error {
	checks := []struct { name string; cmd string }{{"Fcitx5", "fcitx5"}, {"Fcitx5 Rime", "rime_deployer"}, {"Fcitx5 remote", "fcitx5-remote"}}
	failed := false
	for _, check := range checks {
		path, err := exec.LookPath(check.cmd)
		if err != nil { fmt.Fprintf(a.Out, "[缺失] %s (%s)\n", check.name, check.cmd); failed = true } else { fmt.Fprintf(a.Out, "[正常] %s: %s\n", check.name, path) }
	}
	cfg, err := config.Load(); if err != nil { fmt.Fprintf(a.Out, "[未配置] %v\n", err); return err }
	schemaPath := filepath.Join(cfg.RimeUserDir, cfg.SchemaID+".schema.yaml")
	if _, err := os.Stat(schemaPath); err != nil { fmt.Fprintf(a.Out, "[缺失] 输入方案：%s\n", schemaPath); failed = true } else { fmt.Fprintf(a.Out, "[正常] 输入方案：%s\n", schemaPath) }
	if err := remote.New(cfg).Health(ctx); err != nil { fmt.Fprintf(a.Out, "[失败] 服务器：%v\n", err); failed = true } else { fmt.Fprintln(a.Out, "[正常] 服务器健康检查") }
	if failed { return errors.New("环境检查发现问题") }
	return nil
}

func (a *App) Render() error {
	cfg, err := config.Load(); if err != nil { return err }
	st, err := state.Load(); if err != nil { return err }
	count, err := rime.Render(cfg, st.Lexicon); if err != nil { return err }
	fmt.Fprintf(a.Out, "已生成 Rime 个人词典，共 %d 条编码\n", count)
	return nil
}

func (a *App) Deploy() error { cfg, err := config.Load(); if err != nil { return err }; return rime.Deploy(cfg) }
func expandHome(path string) string { if path == "~" || strings.HasPrefix(path, "~/") { if home, err := os.UserHomeDir(); err == nil { return filepath.Join(home, strings.TrimPrefix(path, "~/")) } }; return path }
func firstNonEmpty(values ...string) string { for _, value := range values { if value != "" { return value } }; return "" }
