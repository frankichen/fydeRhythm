package deepseek

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/frankichen/fydeRhythm/ubuntu-client/internal/config"
)

const maxResponseBytes = 1 << 20

type Client struct {
	cfg  config.AIConfig
	http *http.Client
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Thinking    thinking      `json:"thinking"`
	MaxTokens   int           `json:"max_tokens"`
	Temperature float64       `json:"temperature"`
	Stream      bool          `json:"stream"`
}

type thinking struct {
	Type string `json:"type"`
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    any    `json:"code"`
	} `json:"error,omitempty"`
}

func New(cfg config.AIConfig) (*Client, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if !cfg.Enabled {
		return nil, errors.New("DeepSeek AI 尚未启用，请先运行 fyderhythm-sync ai-config")
	}

	transport := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           (&net.Dialer{Timeout: 15 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          10,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   30 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		TLSClientConfig:       &tls.Config{MinVersion: tls.VersionTLS12},
	}
	if strings.TrimSpace(cfg.ProxyURL) != "" {
		proxyURL, err := url.Parse(strings.TrimSpace(cfg.ProxyURL))
		if err != nil {
			return nil, fmt.Errorf("解析 DeepSeek 代理地址: %w", err)
		}
		transport.Proxy = http.ProxyURL(proxyURL)
	}

	return &Client{
		cfg: cfg,
		http: &http.Client{
			Timeout:   time.Duration(cfg.TimeoutSeconds) * time.Second,
			Transport: transport,
		},
	}, nil
}

func (c *Client) Test(ctx context.Context) (string, error) {
	return c.complete(ctx,
		"你是连接测试程序。严格只输出四个汉字：连接成功。不要解释。",
		"请执行连接测试。",
		16,
	)
}

func (c *Client) Correct(ctx context.Context, text string) (string, error) {
	text, err := c.prepareInput(text)
	if err != nil {
		return "", err
	}
	return c.complete(ctx,
		"你是中文输入法纠错器。修正错别字、漏字、明显语病和标点，但必须保持原意、语气、人名、术语、数字、代码和格式。只输出修正后的文本，不要解释，不要加引号，不要使用 Markdown。若无需修改，原样输出。",
		text,
		512,
	)
}

func (c *Client) Predict(ctx context.Context, text string) (string, error) {
	text, err := c.prepareInput(text)
	if err != nil {
		return "", err
	}
	return c.complete(ctx,
		"你是中文输入法的下一句预测器。根据用户已经输入的上下文，预测最自然、最有用的一小段续写。只输出续写内容，不要重复原文，不要解释，不要加引号，不要使用 Markdown。优先控制在 8 到 40 个汉字；上下文若像代码，则续写代码。",
		text,
		256,
	)
}

func (c *Client) prepareInput(text string) (string, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return "", errors.New("没有可发送给 DeepSeek 的文本")
	}
	if utf8.RuneCountInString(text) > c.cfg.MaxInputChars {
		return "", fmt.Errorf("输入文本超过 %d 个字符的隐私和费用上限", c.cfg.MaxInputChars)
	}
	return text, nil
}

func (c *Client) complete(ctx context.Context, systemPrompt, userText string, maxTokens int) (string, error) {
	requestBody := chatRequest{
		Model: c.cfg.Model,
		Messages: []chatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userText},
		},
		Thinking:    thinking{Type: "disabled"},
		MaxTokens:   maxTokens,
		Temperature: 0.2,
		Stream:      false,
	}
	encoded, err := json.Marshal(requestBody)
	if err != nil {
		return "", err
	}

	endpoint := strings.TrimRight(c.cfg.BaseURL, "/") + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(encoded))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "fydeRhythm-Ubuntu/0.2")

	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("调用 DeepSeek 失败: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return "", fmt.Errorf("读取 DeepSeek 响应: %w", err)
	}

	var decoded chatResponse
	if err := json.Unmarshal(body, &decoded); err != nil {
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return "", fmt.Errorf("DeepSeek 返回 HTTP %d", resp.StatusCode)
		}
		return "", fmt.Errorf("解析 DeepSeek 响应: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		message := "请求失败"
		if decoded.Error != nil && strings.TrimSpace(decoded.Error.Message) != "" {
			message = decoded.Error.Message
		}
		return "", fmt.Errorf("DeepSeek 返回 HTTP %d: %s", resp.StatusCode, message)
	}
	if decoded.Error != nil {
		return "", errors.New(decoded.Error.Message)
	}
	if len(decoded.Choices) == 0 {
		return "", errors.New("DeepSeek 没有返回候选内容")
	}
	result := cleanResult(decoded.Choices[0].Message.Content)
	if result == "" {
		return "", errors.New("DeepSeek 返回了空内容")
	}
	return result, nil
}

func cleanResult(value string) string {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "```") && strings.HasSuffix(value, "```") {
		lines := strings.Split(value, "\n")
		if len(lines) >= 3 {
			value = strings.Join(lines[1:len(lines)-1], "\n")
			value = strings.TrimSpace(value)
		}
	}
	if len(value) >= 2 {
		pairs := [][2]string{{"\"", "\""}, {"“", "”"}, {"‘", "’"}}
		for _, pair := range pairs {
			if strings.HasPrefix(value, pair[0]) && strings.HasSuffix(value, pair[1]) {
				value = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(value, pair[0]), pair[1]))
				break
			}
		}
	}
	return value
}
