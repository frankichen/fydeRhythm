package config

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

const (
	ModeFull         = "full"
	ModeDownloadOnly = "download-only"
	ModeManual       = "manual"
)

type Config struct {
	ServerURL          string `json:"server_url"`
	APIToken           string `json:"api_token"`
	DeviceID           string `json:"device_id"`
	DeviceName         string `json:"device_name"`
	Mode               string `json:"mode"`
	SchemaID           string `json:"schema_id"`
	RimeUserDir        string `json:"rime_user_dir"`
	SyncIntervalMinute int    `json:"sync_interval_minutes"`
}

func Default() (Config, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Config{}, err
	}
	deviceID, err := randomID("ubuntu")
	if err != nil {
		return Config{}, err
	}
	hostname, _ := os.Hostname()
	if strings.TrimSpace(hostname) == "" {
		hostname = "Ubuntu"
	}
	return Config{
		ServerURL:          "https://shulufa.555044.xyz",
		DeviceID:           deviceID,
		DeviceName:         hostname,
		Mode:               ModeDownloadOnly,
		SchemaID:           "wubi86_jidian_pinyin_smart",
		RimeUserDir:        filepath.Join(home, ".local", "share", "fcitx5", "rime"),
		SyncIntervalMinute: 5,
	}, nil
}

func Paths() (configPath, statePath, lockPath string, err error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", "", "", err
	}
	dir := filepath.Join(base, "fyderhythm")
	return filepath.Join(dir, "config.json"), filepath.Join(dir, "state.json"), filepath.Join(dir, "sync.lock"), nil
}

func Load() (Config, error) {
	configPath, _, _, err := Paths()
	if err != nil {
		return Config{}, err
	}
	data, err := os.ReadFile(configPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Config{}, fmt.Errorf("尚未初始化，请先运行 fyderhythm-sync init")
		}
		return Config{}, err
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("解析配置: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func Save(cfg Config) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	configPath, _, _, err := Paths()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(configPath), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	tmp := configPath + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	if err := os.Chmod(tmp, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, configPath)
}

func (c Config) Validate() error {
	if strings.TrimSpace(c.ServerURL) == "" {
		return errors.New("server_url 不能为空")
	}
	u, err := url.Parse(c.ServerURL)
	if err != nil || u.Host == "" {
		return errors.New("server_url 格式不正确")
	}
	local := u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1" || u.Hostname() == "::1"
	if u.Scheme != "https" && !local {
		return errors.New("远程服务器必须使用 HTTPS")
	}
	if len(strings.TrimSpace(c.APIToken)) < 24 {
		return errors.New("API Token 至少需要 24 个字符")
	}
	if strings.TrimSpace(c.DeviceID) == "" || len(c.DeviceID) > 128 {
		return errors.New("device_id 不正确")
	}
	switch c.Mode {
	case ModeFull, ModeDownloadOnly, ModeManual:
	default:
		return fmt.Errorf("不支持的同步模式 %q", c.Mode)
	}
	if strings.TrimSpace(c.SchemaID) == "" {
		return errors.New("schema_id 不能为空")
	}
	if strings.TrimSpace(c.RimeUserDir) == "" {
		return errors.New("rime_user_dir 不能为空")
	}
	return nil
}

func randomID(prefix string) (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return prefix + "-" + hex.EncodeToString(buf), nil
}

func NewChangeID() (string, error) { return randomID("chg") }
func NewEntryID() (string, error)  { return randomID("lex") }
