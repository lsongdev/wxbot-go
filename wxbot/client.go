// Package wechatbot provides a Go SDK for the Weixin iLink Bot API.
package wxbot

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
	config *Config
	client *http.Client
	// contextTokens caches the latest context_token per user ID.
	// Populated automatically by Start; used by Push for proactive sends.
	// contextTokens sync.Map // map[string]string
	// config is the bot configuration.
	// handler Handler
}

// NewClient creates a Client with the given configuration.
// Pass nil or empty config for defaults; set Token after LoginWithQR.
func NewBot(cfg *Config) *Client {
	c := &Client{
		config: cfg,
		client: http.DefaultClient,
	}
	return c
}

// Config returns the bot configuration.
func randomWechatUIN() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	n := binary.BigEndian.Uint32(b)
	return base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%d", n)))
}

// BaseInfo is attached to every outgoing API request.
type BaseInfo struct {
	ChannelVersion string `json:"channel_version,omitempty"`
}

func (c *Client) buildBaseInfo() *BaseInfo {
	return &BaseInfo{ChannelVersion: "1.0.0"}
}

func (c *Client) request(method string, path string, body any, out any) (err error) {
	url := DefaultBaseURL + path
	data, err := json.Marshal(body)
	if err != nil {
		err = fmt.Errorf("marshal request: %w", err)
		return
	}
	req, err := http.NewRequest(method, url, bytes.NewReader(data))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Content-Length", fmt.Sprintf("%d", len(data)))
	req.Header.Set("X-WECHAT-UIN", randomWechatUIN())
	req.Header.Set("AuthorizationType", "ilink_bot_token")
	req.Header.Set("Authorization", "Bearer "+c.config.Token)
	resp, err := c.client.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		err = fmt.Errorf("read response: %w", err)
		return
	}
	if resp.StatusCode >= 400 {
		err = fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(respBody))
		return
	}
	var base BaseResponse
	err = json.Unmarshal(respBody, &base)
	if base.ErrCode != 0 {
		err = fmt.Errorf(base.ErrMsg)
		return
	}
	err = json.Unmarshal(respBody, &out)
	return
}

// GetUpdates performs a long-poll request to receive new messages.
// Returns an empty response (not an error) on client-side timeout.
func (c *Client) GetUpdates(updateBuf string) (resp *GetUpdatesResp, err error) {
	reqBody := GetUpdatesReq{
		GetUpdatesBuf: updateBuf,
		BaseInfo:      c.buildBaseInfo(),
	}
	// Add buffer over server-side long-poll timeout
	// timeout := defaultLongPollTimeout + 5*time.Second
	err = c.request(http.MethodPost, "/ilink/bot/getupdates", reqBody, &resp)
	return
}

func NewMessage(contextToken, to string, items ...MessageItem) (message *Message) {
	clientID := fmt.Sprintf("sdk-%d", time.Now().UnixMilli())
	message = &Message{
		ToUserID:     to,
		ClientID:     clientID,
		MessageType:  MsgTypeBot,
		MessageState: StateFinish,
		ContextToken: contextToken,
		ItemList:     items,
	}
	return
}

// SendMessage sends a raw message request.
func (c *Client) SendMessage(message *Message) (err error) {
	req := &SendMessageReq{
		BaseInfo: c.buildBaseInfo(),
		Message:  message,
	}
	err = c.request(http.MethodPost, "/ilink/bot/sendmessage", req, nil)
	return
}

// GetConfig fetches bot config (includes typing_ticket) for a given user.
func (c *Client) GetConfig(contextToken, userID string) (resp *GetConfigResp, err error) {
	req := GetConfigReq{
		ILinkUserID:  userID,
		ContextToken: contextToken,
		BaseInfo:     c.buildBaseInfo(),
	}
	err = c.request(http.MethodPost, "/ilink/bot/getconfig", req, &resp)
	return
}

// SendTyping sends or cancels a typing indicator for a user.
func (c *Client) SendTyping(userID, typingTicket string, status TypingStatus) (err error) {
	reqBody := SendTypingReq{
		ILinkUserID:  userID,
		TypingTicket: typingTicket,
		Status:       status,
		BaseInfo:     c.buildBaseInfo(),
	}
	err = c.request(http.MethodPost, "/ilink/bot/sendtyping", reqBody, nil)
	return
}

// GetUploadURL requests a pre-signed CDN upload URL.
func (c *Client) GetUploadURL(req *GetUploadURLReq) (resp *GetUploadURLResp, err error) {
	req.BaseInfo = c.buildBaseInfo()
	err = c.request(http.MethodPost, "/ilink/bot/getuploadurl", req, &resp)
	return
}

// ErrNoContextToken is returned by Push when no cached context token exists for the user.
var ErrNoContextToken = fmt.Errorf("no cached context token for this user; user must send a message first")

// ExtractText is a helper to extract the first text body from a message's item list.
func ExtractText(msg *Message) string {
	for _, item := range msg.ItemList {
		if item.Type == MessageItemText && item.TextItem != nil {
			return item.TextItem.Text
		}
	}
	return ""
}

// Start runs a long-poll loop, calling the handler for each inbound message.
// Blocks until ctx is cancelled. Handles retries and backoff automatically.
func (c *Client) Start(ctx context.Context, onMessage MessageHandler) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		resp, err := c.GetUpdates(c.config.UpdatesBuf)
		if err != nil {
			continue
		}
		if resp.GetUpdatesBuf != "" {
			c.config.UpdatesBuf = resp.GetUpdatesBuf
		}
		for _, msg := range resp.Messages {
			onMessage(&ReplyMessage{
				c:       c,
				Message: &msg,
			})
		}
	}
}

type MessageHandler func(message *ReplyMessage)

type ReplyMessage struct {
	c *Client
	*Message
}

func (r *ReplyMessage) Typing(status TypingStatus) (err error) {
	userID := r.Message.FromUserID
	resp, err := r.c.GetConfig(r.Message.ContextToken, userID)
	if err != nil {
		return
	}
	err = r.c.SendTyping(userID, resp.TypingTicket, status)
	return
}

func (r *ReplyMessage) Reply(items ...MessageItem) error {
	message := NewMessage(r.Message.ContextToken, r.Message.FromUserID, items...)
	return r.c.SendMessage(message)
}

func (r *ReplyMessage) ReplyText(content string) error {
	return r.Reply(MessageItem{
		Type: MessageItemText,
		TextItem: &TextItem{
			Text: content,
		},
	})
}
