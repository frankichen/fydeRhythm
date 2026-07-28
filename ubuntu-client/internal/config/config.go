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

	DefaultDeepSeekBaseURL = "https://api.deepseek.com"
	DefaultDeepSeekModel   = "deepseek-v4-flash"
)

type AIConfig struct {
	Enabled        bool   `json:"enabled"`
	BaseURL        string `json:"base_url"`
	APIKey         string `json:"api_key"`
	Model          string `json:"model"`
	ProxyURL       string `json:"proxy_url,omitempty"`
	TimeoutSeconds int    `json:"timeout_seconds"`
	MaxInputChars  int    `json:"max_input_chars"`
}

type Config struct {
	ServerURL          string   `json:"server_url"`
	APIToken           string   `json:"api_token"`
	DeviceID           string   `json:"device_id"`
	DeviceName         string   `json:"device_name"`
	Mode               string   `json:"mode"`
	SchemaID           string   `json:"schema_id"`
	RimeUserDir        string   `json:"rime_user_dir"`
	SyncIntervalMinute int      `json:"sync_interval_minutes"`
	AI                 AIConfig `json:"ai"`
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
	cfg := Config{
		ServerURL:          "https://shulufa.555044.xyz",
		DeviceID:           deviceID,
		DeviceName:         hostname,
		Mode:               ModeDownloadOnly,
		SchemaID:           "wubi86_jidian_pinyin_smart",
		RimeUserDir:        filepath.Join(home, ".local", "share", "fcitx5", "rime"),
		SyncIntervalMinute: 5,
	}
	applyDefaults(&cfg)
	return cfg, nil
}

func applyDefaults(cfg *Config) {
	if cfg.SyncIntervalMinute <= 0 {
		cfg.SyncIntervalMinute = 5
	}
	if strings.TrimSpace(cfg.AI.BaseURL) == "" {
		cfg.AI.BaseURL = DefaultDeepSeekBaseURL
	}
	if strings.TrimSpace(cfg.AI.Model) == "" {
		cfg.AI.Model = DefaultDeepSeekModel
	}
	if cfg.AI.TimeoutSeconds <= 0 {
		cfg.AI.TimeoutSeconds = 45
	}
	if cfg.AI.MaxInputChars <= 0 {
		cfg.AI.MaxInputChars = 2000
	}
}

func Paths() (configPath, statePath, lockPath string, err error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", "", "", err
	}
	dir := filepath.Join(base, "fyderhythm")
	return filepath.Join(dir, "config.json"), filepath.Join(dir, "state.json"), filepath.Join(dir, "sync.lock"), nil
}

func RuntimeSocketPath() string {
	if runtimeDir := strings.TrimSpace(os.Getenv("XDG_RUNTIME_DIR")); runtimeDir != "" {
		return filepath.Join(runtimeDir, "fyderhythm-ai.sock")
	}
	return filepath.Join(os.TempDir(), fmt.Sprintf("fyderhythm-ai-%d.sock", os.Getuid()))
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
	applyDefaults(&cfg)
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func Save(cfg Config) error {
	applyDefaults(&cfg)
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
	return c.AI.Validate()
}

func (c AIConfig) Validate() error {
	if !c.Enabled && strings.TrimSpace(c.APIKey) == "" {
		return nil
	}
	if len(strings.TrimSpace(c.APIKey)) < 16 {
		return errors.New("DeepSeek API Key 长度不正确")
	}
	u, err := url.Parse(strings.TrimSpace(c.BaseURL))
	local := u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1" || u.Hostname() == "::1"
	if err != nil || u.Host == "" || (u.Scheme != "https" && !(local && u.Scheme == "http")) {
		return errors.New("DeepSeek base_url 必须是有效的 HTTPS 地址；仅本机测试允许 HTTP")
	}
	if strings.TrimSpace(c.Model) == "" {
		return errors.New("DeepSeek model 不能为空")
	}
	if c.TimeoutSeconds < 5 || c.TimeoutSeconds > 180 {
		return errors.New("DeepSeek timeout_seconds 必须在 5 到 180 之间")
	}
	if c.MaxInputChars < 100 || c.MaxInputChars > 20000 {
		return errors.New("DeepSeek max_input_chars 必须在 100 到 20000 之间")
	}
	if strings.TrimSpace(c.ProxyURL) != "" {
		proxy, err := url.Parse(strings.TrimSpace(c.ProxyURL))
		if err != nil || proxy.Host == "" || (proxy.Scheme != "http" && proxy.Scheme != "https") {
			return errors.New("DeepSeek proxy_url 仅支持 http:// 或 https:// 代理")
		}
	}
	return nil
}

func MaskSecret(value string) string {
	value = strings.TrimSpace(value)
	if len(value) <= 8 {
		if value == "" {
			return "未设置"
		}
		return "********"
	}
	return value[:4] + "…" + value[len(value)-4:]
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
