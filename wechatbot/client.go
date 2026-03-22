// Package wechatbot provides a Go SDK for the Weixin iLink Bot API.
package wechatbot

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	DefaultBaseURL    = "https://ilinkai.weixin.qq.com"
	DefaultCDNBaseURL = "https://novac2c.cdn.weixin.qq.com/c2c"
	DefaultBotType    = "3"

	defaultLongPollTimeout = 35 * time.Second
	defaultAPITimeout      = 15 * time.Second
)

// Client communicates with the Weixin iLink Bot API.
type Client struct {
	BaseURL    string
	CDNBaseURL string
	Token      string
	BotType    string
	Version    string
	HTTPClient *http.Client

	// contextTokens caches the latest context_token per user ID.
	// Populated automatically by Start; used by Push for proactive sends.
	contextTokens sync.Map // map[string]string

	// config is the bot configuration.
	config *Config
}

// NewClient creates a Client with the given configuration.
// Pass nil or empty config for defaults; set Token after LoginWithQR.
func NewClient(cfg *Config) *Client {
	if cfg == nil {
		cfg = &Config{}
	}

	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	cdnBaseURL := cfg.CDNBaseURL
	if cdnBaseURL == "" {
		cdnBaseURL = DefaultCDNBaseURL
	}
	botType := cfg.BotType
	if botType == "" {
		botType = DefaultBotType
	}
	version := cfg.Version
	if version == "" {
		version = "1.0.0"
	}

	c := &Client{
		BaseURL:    baseURL,
		CDNBaseURL: cdnBaseURL,
		Token:      cfg.Token,
		BotType:    botType,
		Version:    version,
		HTTPClient: &http.Client{},
		config:     cfg,
	}

	// Set handler's client if provided
	if cfg.Handler != nil {
		cfg.Handler.SetClient(c)
	}

	return c
}

// Config returns the bot configuration.
func (c *Client) Config() *Config {
	return c.config
}

func ensureTrailingSlash(u string) string {
	if strings.HasSuffix(u, "/") {
		return u
	}
	return u + "/"
}

func randomWechatUIN() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	n := binary.BigEndian.Uint32(b)
	return base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%d", n)))
}

func (c *Client) buildBaseInfo() *BaseInfo {
	return &BaseInfo{ChannelVersion: c.Version}
}

func (c *Client) doPost(ctx context.Context, endpoint string, body any, timeout time.Duration) ([]byte, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	base := ensureTrailingSlash(c.BaseURL)
	u, err := url.JoinPath(base, endpoint)
	if err != nil {
		return nil, fmt.Errorf("build url: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("AuthorizationType", "ilink_bot_token")
	req.Header.Set("Content-Length", fmt.Sprintf("%d", len(data)))
	req.Header.Set("X-WECHAT-UIN", randomWechatUIN())
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(respBody))
	}
	return respBody, nil
}

func (c *Client) doGet(ctx context.Context, rawURL string, extraHeaders map[string]string, timeout time.Duration) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	for k, v := range extraHeaders {
		req.Header.Set(k, v)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}
	return body, nil
}

// GetUpdates performs a long-poll request to receive new messages.
// Returns an empty response (not an error) on client-side timeout.
func (c *Client) GetUpdates(ctx context.Context, getUpdatesBuf string) (*GetUpdatesResp, error) {
	reqBody := GetUpdatesReq{
		GetUpdatesBuf: getUpdatesBuf,
		BaseInfo:      c.buildBaseInfo(),
	}
	// Add buffer over server-side long-poll timeout
	timeout := defaultLongPollTimeout + 5*time.Second

	data, err := c.doPost(ctx, "ilink/bot/getupdates", reqBody, timeout)
	if err != nil {
		if ctx.Err() != nil {
			// Context cancelled or parent timeout — not a poll timeout
			return nil, ctx.Err()
		}
		// Client-side timeout is normal for long-poll
		return &GetUpdatesResp{Ret: 0, GetUpdatesBuf: getUpdatesBuf}, nil
	}

	var resp GetUpdatesResp
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("unmarshal getUpdates: %w", err)
	}
	return &resp, nil
}

// SendMessage sends a raw message request.
func (c *Client) SendMessage(ctx context.Context, msg *SendMessageReq) error {
	msg.BaseInfo = c.buildBaseInfo()
	_, err := c.doPost(ctx, "ilink/bot/sendmessage", msg, defaultAPITimeout)
	return err
}

// SendText sends a plain text message to a user.
// contextToken must come from the inbound message's ContextToken field.
func (c *Client) SendText(ctx context.Context, to, text, contextToken string) (string, error) {
	clientID := fmt.Sprintf("sdk-%d", time.Now().UnixMilli())
	msg := &SendMessageReq{
		Msg: &WeixinMessage{
			ToUserID:     to,
			ClientID:     clientID,
			MessageType:  MsgTypeBot,
			MessageState: StateFinish,
			ContextToken: contextToken,
			ItemList: []MessageItem{
				{
					Type:     ItemText,
					TextItem: &TextItem{Text: text},
				},
			},
		},
	}
	if err := c.SendMessage(ctx, msg); err != nil {
		return "", err
	}
	return clientID, nil
}

// GetConfig fetches bot config (includes typing_ticket) for a given user.
func (c *Client) GetConfig(ctx context.Context, userID, contextToken string) (*GetConfigResp, error) {
	reqBody := GetConfigReq{
		ILinkUserID:  userID,
		ContextToken: contextToken,
		BaseInfo:     c.buildBaseInfo(),
	}
	data, err := c.doPost(ctx, "ilink/bot/getconfig", reqBody, 10*time.Second)
	if err != nil {
		return nil, err
	}
	var resp GetConfigResp
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("unmarshal getConfig: %w", err)
	}
	return &resp, nil
}

// SendTyping sends or cancels a typing indicator for a user.
func (c *Client) SendTyping(ctx context.Context, userID, typingTicket string, status TypingStatus) error {
	reqBody := SendTypingReq{
		ILinkUserID:  userID,
		TypingTicket: typingTicket,
		Status:       status,
		BaseInfo:     c.buildBaseInfo(),
	}
	_, err := c.doPost(ctx, "ilink/bot/sendtyping", reqBody, 10*time.Second)
	return err
}

// GetUploadURL requests a pre-signed CDN upload URL.
func (c *Client) GetUploadURL(ctx context.Context, req *GetUploadURLReq) (*GetUploadURLResp, error) {
	req.BaseInfo = c.buildBaseInfo()
	data, err := c.doPost(ctx, "ilink/bot/getuploadurl", req, defaultAPITimeout)
	if err != nil {
		return nil, err
	}
	var resp GetUploadURLResp
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("unmarshal getUploadUrl: %w", err)
	}
	return &resp, nil
}

// SetContextToken caches a context token for a user.
// Called automatically by Monitor; can also be called manually
// if you obtain a token through other means.
func (c *Client) SetContextToken(userID, token string) {
	c.contextTokens.Store(userID, token)
}

// GetContextToken returns the cached context token for a user, if any.
func (c *Client) GetContextToken(userID string) (string, bool) {
	v, ok := c.contextTokens.Load(userID)
	if !ok {
		return "", false
	}
	return v.(string), true
}

// Push sends a proactive text message to a user using a cached context token.
// The user must have previously sent a message (so a context token is cached).
// Returns ErrNoContextToken if no cached token is available.
func (c *Client) Push(ctx context.Context, to, text string) (string, error) {
	token, ok := c.GetContextToken(to)
	if !ok {
		return "", ErrNoContextToken
	}
	return c.SendText(ctx, to, text, token)
}

// ErrNoContextToken is returned by Push when no cached context token exists for the user.
var ErrNoContextToken = fmt.Errorf("no cached context token for this user; user must send a message first")

// ExtractText is a helper to extract the first text body from a message's item list.
func ExtractText(msg *WeixinMessage) string {
	for _, item := range msg.ItemList {
		if item.Type == ItemText && item.TextItem != nil {
			return item.TextItem.Text
		}
	}
	return ""
}
