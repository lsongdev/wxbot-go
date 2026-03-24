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
	"log"
	"net/http"
	"time"
)

const (
	// DefaultBaseURL    = "https://ilinkai.weixin.qq.com"
	// DefaultCDNBaseURL = "https://novac2c.cdn.weixin.qq.com/c2c"
	DefaultBotType        = "3"
	DefaultChannelVersion = "1.0.0"

	defaultLongPollTimeout = 35 * time.Second
	defaultAPITimeout      = 15 * time.Second
	defaultLoginTimeout    = 8 * time.Minute
	maxQRRefreshCount      = 3
	qrLongPollTimeout      = 35 * time.Second
)

// Client communicates with the Weixin iLink Bot API.
type WeChatBot struct {
	*Config
	*http.Client
}

// NewClient creates a Client with the given configuration.
// Pass nil or empty config for defaults; set Token after LoginWithQR.
func NewBot(cfg *Config) *WeChatBot {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://ilinkai.weixin.qq.com"
	}
	if cfg.CDNBaseURL == "" {
		cfg.CDNBaseURL = "https://novac2c.cdn.weixin.qq.com/c2c"
	}
	c := &WeChatBot{
		Config: cfg,
		Client: http.DefaultClient,
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

func (c *WeChatBot) buildBaseInfo() *BaseInfo {
	return &BaseInfo{ChannelVersion: DefaultChannelVersion}
}

func (c *WeChatBot) request(method string, path string, body any, out any, options ...any) (err error) {
	url := c.BaseURL + path
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
	req.Header.Set("Authorization", "Bearer "+c.Token)
	resp, err := c.Client.Do(req)
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
	var base WeChatBotError
	err = json.Unmarshal(respBody, &base)
	if base.ErrCode != 0 {
		err = base
		return
	}
	err = json.Unmarshal(respBody, &out)
	return
}

// FetchQRCode requests a new login QR code from the API.
//
//	{
//	    "qrcode": "b73e84f617fe08e311355ef30255039b",
//	    "qrcode_img_content": "https://liteapp.weixin.qq.com/q/7GiQu1?qrcode=b73e84f617fe08e311355ef30255039b&bot_type=3",
//	    "ret": 0
//	}
func (c *WeChatBot) GetBotQRCode() (resp *QRCodeResponse, err error) {
	path := fmt.Sprintf("/ilink/bot/get_bot_qrcode?bot_type=%s", DefaultBotType)
	err = c.request(http.MethodGet, path, nil, &resp)
	return
}

// PollQRStatus polls the status of a QR code login.
//
//	{
//	    "baseurl": "https://ilinkai.weixin.qq.com",
//	    "bot_token": "f7a1cf7ca655@im.bot:0600007f18ae828ed54c6eb716faa0e2bfbbc5",
//	    "ilink_bot_id": "f7a1cf7ca655@im.bot",
//	    "ilink_user_id": "o9cq80_M7bAJtIt_k1O9Ev6vCBHQ@im.wechat",
//	    "ret": 0,
//	    "status": "confirmed"
//	}
func (c *WeChatBot) GetQRCodeStatus(qrcode string) (resp *QRStatusResponse, err error) {
	path := fmt.Sprintf("/ilink/bot/get_qrcode_status?qrcode=%s", qrcode)
	// headers := map[string]string{
	// 	"iLink-App-ClientVersion": "1",
	// }
	err = c.request(http.MethodGet, path, nil, &resp)
	return
}

func (c *WeChatBot) WaitingForLogin(ctx context.Context, qrcode string) (resp *QRStatusResponse, err error) {
	for {
		select {
		case <-ctx.Done():
			return resp, ctx.Err()
		default:
		}
		resp, err = c.GetQRCodeStatus(qrcode)
		if err != nil {
			return
		}
		switch resp.Status {
		case "wait":
			continue
		case "scaned":
			continue
		case "confirmed":
			return
		case "expired":
		default:
			err = fmt.Errorf("qrcode %s", resp.Status)
			return
		}
	}
}

func (c *WeChatBot) Login(ctx context.Context, force bool) (botToken string, err error) {
	if c.Token == "" || force {
		qrcode, err := c.GetBotQRCode()
		if err != nil {
			return botToken, err
		}
		log.Println("WeChat QRCode:", qrcode.QRCodeImgContent)
		resp, err := c.WaitingForLogin(ctx, qrcode.QRCode)
		if err != nil {
			return resp.BotToken, err
		}
		log.Println("WeChat Login:", resp.ILinkBotID)
		c.Token = resp.BotToken
	}
	return
}

// GetUpdates performs a long-poll request to receive new messages.
// Returns an empty response (not an error) on client-side timeout.
func (c *WeChatBot) GetUpdates(updateBuf string) (resp *GetUpdatesResp, err error) {
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

type SendMessageResp struct{}

// SendMessage sends a raw message request.
func (c *WeChatBot) SendMessage(message *Message) (resp *SendMessageResp, err error) {
	req := &SendMessageReq{
		BaseInfo: c.buildBaseInfo(),
		Message:  message,
	}
	err = c.request(http.MethodPost, "/ilink/bot/sendmessage", req, &resp)
	return
}

// GetConfig fetches bot config (includes typing_ticket) for a given user.
func (c *WeChatBot) GetConfig(contextToken, userID string) (resp *GetConfigResp, err error) {
	req := GetConfigReq{
		ILinkUserID:  userID,
		ContextToken: contextToken,
		BaseInfo:     c.buildBaseInfo(),
	}
	err = c.request(http.MethodPost, "/ilink/bot/getconfig", req, &resp)
	return
}

// SendTyping sends or cancels a typing indicator for a user.
func (c *WeChatBot) SendTyping(typingTicket string, userID string, status TypingStatus) (err error) {
	reqBody := SendTypingReq{
		TypingTicket: typingTicket,
		ILinkUserID:  userID,
		Status:       status,
		BaseInfo:     c.buildBaseInfo(),
	}
	err = c.request(http.MethodPost, "/ilink/bot/sendtyping", reqBody, nil)
	return
}

// GetUploadURL requests a pre-signed CDN upload URL.
func (c *WeChatBot) GetUploadURL(req *GetUploadURLReq) (resp *GetUploadURLResp, err error) {
	req.BaseInfo = c.buildBaseInfo()
	err = c.request(http.MethodPost, "/ilink/bot/getuploadurl", req, &resp)
	return
}

// ErrNoContextToken is returned by Push when no cached context token exists for the user.
var ErrNoContextToken = fmt.Errorf("no cached context token for this user; user must send a message first")

// Start runs a long-poll loop, calling the handler for each inbound message.
// Blocks until ctx is cancelled. Handles retries and backoff automatically.
func (c *WeChatBot) Start(ctx context.Context, onMessage MessageHandler) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		resp, err := c.GetUpdates(c.UpdatesBuf)
		if err != nil {
			return err
		}
		c.UpdatesBuf = resp.GetUpdatesBuf
		for _, msg := range resp.Messages {
			onMessage(&ReplyMessage{
				WeChatBot: c,
				Message:   &msg,
			})
		}
	}
}

type MessageHandler func(message *ReplyMessage)

type ReplyMessage struct {
	*WeChatBot
	*Message
}

func (r *ReplyMessage) Typing(status TypingStatus) (err error) {
	resp, err := r.GetConfig(r.ContextToken, r.FromUserID)
	if err != nil {
		return
	}
	err = r.SendTyping(resp.TypingTicket, r.FromUserID, status)
	return
}

func (r *ReplyMessage) Reply(items ...MessageItem) (*SendMessageResp, error) {
	message := NewMessage(r.ContextToken, r.FromUserID, items...)
	return r.SendMessage(message)
}

func (r *ReplyMessage) ReplyText(content string) (*SendMessageResp, error) {
	return r.Reply(MessageItem{
		Type: MessageItemText,
		TextItem: &TextItem{
			Text: content,
		},
	})
}

// SendImage uploads and sends an image message.
// imageData is the raw image bytes, fileName is the image filename.
func (c *WeChatBot) SendImage(contextToken, toUserID string, imageData []byte, fileName string) (*SendMessageResp, error) {
	uploadResult, err := c.UploadFile(MediaImage, toUserID, imageData, fileName, true)
	if err != nil {
		return nil, fmt.Errorf("upload image: %w", err)
	}
	message := NewMessage(contextToken, toUserID, MessageItem{
		Type: MessageItemImage,
		ImageItem: &ImageItem{
			Media:   uploadResult.CDNMedia,
			MidSize: uploadResult.FileSize,
		},
	})
	return c.SendMessage(message)
}

// SendFile uploads and sends a file message.
// fileData is the raw file bytes, fileName is the file filename.
func (c *WeChatBot) SendFile(contextToken, toUserID string, fileData []byte, fileName string) (*SendMessageResp, error) {
	uploadResult, err := c.UploadFile(MediaFile, toUserID, fileData, fileName, true)
	if err != nil {
		return nil, fmt.Errorf("upload file: %w", err)
	}
	message := NewMessage(contextToken, toUserID, MessageItem{
		Type: MessageItemFile,
		FileItem: &FileItem{
			FileName: fileName,
			Media:    uploadResult.CDNMedia,
			Len:      fmt.Sprintf("%d", uploadResult.RawSize),
			MD5:      fmt.Sprintf("%x", md5Sum(fileData)),
		},
	})
	return c.SendMessage(message)
}

// SendVideo uploads and sends a video message.
// videoData is the raw video bytes, fileName is the video filename.
// thumbData can be nil to skip thumbnail.
func (c *WeChatBot) SendVideo(contextToken, toUserID string, videoData []byte, fileName string, thumbData []byte) (*SendMessageResp, error) {
	// Upload main video
	uploadResult, err := c.UploadFile(MediaVideo, toUserID, videoData, fileName, thumbData == nil)
	if err != nil {
		return nil, fmt.Errorf("upload video: %w", err)
	}
	videoItem := &VideoItem{
		Media:      uploadResult.CDNMedia,
		VideoSize:  uploadResult.FileSize,
		PlayLength: 0, // Unknown without parsing video
		VideoMD5:   fmt.Sprintf("%x", md5Sum(videoData)),
	}

	// Upload thumbnail if provided
	if thumbData != nil {
		thumbResult, err := c.UploadFile(MediaVideo, toUserID, thumbData, "thumb_"+fileName, true)
		if err != nil {
			return nil, fmt.Errorf("upload video thumbnail: %w", err)
		}
		videoItem.ThumbMedia = thumbResult.CDNMedia
		videoItem.ThumbSize = thumbResult.FileSize
	}
	message := NewMessage(contextToken, toUserID, MessageItem{
		Type:      MessageItemVideo,
		VideoItem: videoItem,
	})
	return c.SendMessage(message)
}

// ReplyImage uploads and sends an image message as a reply.
func (r *ReplyMessage) ReplyImage(imageData []byte, fileName string) (*SendMessageResp, error) {
	return r.WeChatBot.SendImage(r.ContextToken, r.FromUserID, imageData, fileName)
}

// ReplyFile uploads and sends a file message as a reply.
func (r *ReplyMessage) ReplyFile(fileData []byte, fileName string) (*SendMessageResp, error) {
	return r.WeChatBot.SendFile(r.ContextToken, r.FromUserID, fileData, fileName)
}

// ReplyVideo uploads and sends a video message as a reply.
func (r *ReplyMessage) ReplyVideo(videoData []byte, fileName string, thumbData []byte) (*SendMessageResp, error) {
	return r.WeChatBot.SendVideo(r.ContextToken, r.FromUserID, videoData, fileName, thumbData)
}
