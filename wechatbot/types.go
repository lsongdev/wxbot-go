package wechatbot

import (
	"context"
	"encoding/json"
	"os"
)

// BaseInfo is attached to every outgoing API request.
type BaseInfo struct {
	ChannelVersion string `json:"channel_version,omitempty"`
}

// Handler interface for handling all wechatbot events.
type Handler interface {
	SetClient(c *Client)
	Client() *Client
	OnQRCode(qrcodeURL string)
	OnScanned()
	OnExpired(attempt, maxAttempts int)
	OnLoginResult(result *LoginResult)
	OnMessage(ctx context.Context, msg WeixinMessage)
	OnError(err error)
	OnBufUpdate(buf string)
	OnSessionExpired()
}

// DefaultHandler provides a default implementation of the Handler interface.
// Embed this struct and override only the methods you need.
type DefaultHandler struct {
	client *Client

	// FuncOnQRCode is called when a QR code URL is available for scanning.
	// If nil, the default no-op method is used.
	FuncOnQRCode func(qrcodeURL string)

	// FuncOnScanned is called once when the user has scanned the QR code.
	// If nil, the default no-op method is used.
	FuncOnScanned func()

	// FuncOnExpired is called when the QR code expires and a new one will be fetched.
	// If nil, the default no-op method is used.
	FuncOnExpired func(attempt, maxAttempts int)

	// FuncOnLoginResult is called when the login process completes.
	// If nil, the default no-op method is used.
	FuncOnLoginResult func(result *LoginResult)

	// FuncOnMessage is called for each inbound message from getUpdates.
	// If nil, the default no-op method is used.
	FuncOnMessage func(ctx context.Context, msg WeixinMessage)

	// FuncOnError is called on non-fatal poll errors during monitoring.
	// If nil, the default no-op method is used.
	FuncOnError func(err error)

	// FuncOnBufUpdate is called whenever a new sync cursor is received.
	// Use this to persist the buffer for resuming after restart.
	// If nil, the buffer is only stored in Config.
	FuncOnBufUpdate func(buf string)

	// FuncOnSessionExpired is called when server returns errcode -14.
	// If nil, the default no-op method is used.
	FuncOnSessionExpired func()
}

// SetClient sets the client instance for the handler.
func (h *DefaultHandler) SetClient(c *Client) {
	h.client = c
}

// Client returns the client instance associated with this handler.
func (h *DefaultHandler) Client() *Client {
	return h.client
}

// OnQRCode is called when a QR code URL is available for scanning.
func (h *DefaultHandler) OnQRCode(qrcodeURL string) {
	if h.FuncOnQRCode != nil {
		h.FuncOnQRCode(qrcodeURL)
	}
}

// OnScanned is called once when the user has scanned the QR code.
func (h *DefaultHandler) OnScanned() {
	if h.FuncOnScanned != nil {
		h.FuncOnScanned()
	}
}

// OnExpired is called when the QR code expires and a new one will be fetched.
func (h *DefaultHandler) OnExpired(attempt, maxAttempts int) {
	if h.FuncOnExpired != nil {
		h.FuncOnExpired(attempt, maxAttempts)
	}
}

// OnLoginResult is called when the login process completes (success or failure).
func (h *DefaultHandler) OnLoginResult(result *LoginResult) {
	// Auto-save token on successful login
	if result.Connected && h.client != nil && h.client.config != nil {
		h.client.config.SetToken(result.BotToken)
	}
	// Call user callback if provided
	if h.FuncOnLoginResult != nil {
		h.FuncOnLoginResult(result)
	}
}

// OnMessage is called for each inbound message from getUpdates.
func (h *DefaultHandler) OnMessage(ctx context.Context, msg WeixinMessage) {
	if h.FuncOnMessage != nil {
		h.FuncOnMessage(ctx, msg)
	}
}

// OnError is called on non-fatal poll errors during monitoring.
func (h *DefaultHandler) OnError(err error) {
	if h.FuncOnError != nil {
		h.FuncOnError(err)
	}
}

// OnBufUpdate is called whenever a new sync cursor is received.
func (h *DefaultHandler) OnBufUpdate(buf string) {
	// Always store in config
	if h.client != nil && h.client.config != nil {
		h.client.config.SetBuf(buf)
	}
	// Call user callback if provided
	if h.FuncOnBufUpdate != nil {
		h.FuncOnBufUpdate(buf)
	}
}

// OnSessionExpired is called when server returns errcode -14.
func (h *DefaultHandler) OnSessionExpired() {
	if h.FuncOnSessionExpired != nil {
		h.FuncOnSessionExpired()
	}
}

// Config holds the bot configuration.
type Config struct {
	// Token is the bot token for authentication.
	// Empty for login-only usage; will be auto-updated after LoginWithQR.
	Token string

	// Handler handles all bot events (QR code, messages, etc.).
	Handler Handler

	// BaseURL overrides the default API base URL.
	BaseURL string

	// CDNBBaseURL overrides the default CDN base URL.
	CDNBaseURL string

	// BotType sets the bot type (default: "3").
	BotType string

	// Version sets the client version (default: "1.0.0").
	Version string

	// internal: sync buffer for resuming after restart
	buf string
}

// GetToken returns the current token.
func (c *Config) GetToken() string {
	if c == nil {
		return ""
	}
	return c.Token
}

// SetToken updates the token.
func (c *Config) SetToken(token string) {
	if c == nil {
		return
	}
	c.Token = token
}

// GetBuf returns the current sync buffer.
func (c *Config) GetBuf() string {
	if c == nil {
		return ""
	}
	return c.buf
}

// SetBuf updates the sync buffer.
func (c *Config) SetBuf(buf string) {
	if c == nil {
		return
	}
	c.buf = buf
}

// SetInitialBuf sets the initial sync buffer for resuming after restart.
// Call this before Start() to resume from a previous session.
func (c *Config) SetInitialBuf(buf string) {
	if c == nil {
		return
	}
	c.buf = buf
}

// configData is the JSON-serializable representation of Config.
type configData struct {
	Token      string `json:"token,omitempty"`
	Buf        string `json:"buf,omitempty"`
	BaseURL    string `json:"base_url,omitempty"`
	CDNBaseURL string `json:"cdn_base_url,omitempty"`
	BotType    string `json:"bot_type,omitempty"`
	Version    string `json:"version,omitempty"`
}

// Save persists the config (token and buffer) to a file.
func (c *Config) Save(path string) error {
	if c == nil {
		return nil
	}

	data := configData{
		Token:      c.Token,
		Buf:        c.buf,
		BaseURL:    c.BaseURL,
		CDNBaseURL: c.CDNBaseURL,
		BotType:    c.BotType,
		Version:    c.Version,
	}

	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, b, 0600)
}

// Load restores the config (token and buffer) from a file.
// Returns nil if the file doesn't exist.
func (c *Config) Load(path string) error {
	if c == nil {
		return nil
	}

	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	var data configData
	if err := json.Unmarshal(b, &data); err != nil {
		return err
	}

	c.Token = data.Token
	c.buf = data.Buf
	if data.BaseURL != "" {
		c.BaseURL = data.BaseURL
	}
	if data.CDNBaseURL != "" {
		c.CDNBaseURL = data.CDNBaseURL
	}
	if data.BotType != "" {
		c.BotType = data.BotType
	}
	if data.Version != "" {
		c.Version = data.Version
	}

	return nil
}

// --- Upload ---

type UploadMediaType int

const (
	MediaImage UploadMediaType = 1
	MediaVideo UploadMediaType = 2
	MediaFile  UploadMediaType = 3
	MediaVoice UploadMediaType = 4
)

type GetUploadURLReq struct {
	FileKey       string    `json:"filekey,omitempty"`
	MediaType     int       `json:"media_type,omitempty"`
	ToUserID      string    `json:"to_user_id,omitempty"`
	RawSize       int64     `json:"rawsize,omitempty"`
	RawFileMD5    string    `json:"rawfilemd5,omitempty"`
	FileSize      int64     `json:"filesize,omitempty"`
	ThumbRawSize  int64     `json:"thumb_rawsize,omitempty"`
	ThumbRawMD5   string    `json:"thumb_rawfilemd5,omitempty"`
	ThumbFileSize int64     `json:"thumb_filesize,omitempty"`
	NoNeedThumb   bool      `json:"no_need_thumb,omitempty"`
	AESKey        string    `json:"aeskey,omitempty"`
	BaseInfo      *BaseInfo `json:"base_info,omitempty"`
}

type GetUploadURLResp struct {
	UploadParam      string `json:"upload_param,omitempty"`
	ThumbUploadParam string `json:"thumb_upload_param,omitempty"`
}

// --- Message types ---

type MessageType int

const (
	MsgTypeNone MessageType = 0
	MsgTypeUser MessageType = 1
	MsgTypeBot  MessageType = 2
)

type MessageItemType int

const (
	ItemNone  MessageItemType = 0
	ItemText  MessageItemType = 1
	ItemImage MessageItemType = 2
	ItemVoice MessageItemType = 3
	ItemFile  MessageItemType = 4
	ItemVideo MessageItemType = 5
)

type MessageState int

const (
	StateNew        MessageState = 0
	StateGenerating MessageState = 1
	StateFinish     MessageState = 2
)

type TypingStatus int

const (
	Typing       TypingStatus = 1
	CancelTyping TypingStatus = 2
)

// CDNMedia is a reference to encrypted media on the Weixin CDN.
type CDNMedia struct {
	EncryptQueryParam string `json:"encrypt_query_param,omitempty"`
	AESKey            string `json:"aes_key,omitempty"`
	EncryptType       int    `json:"encrypt_type,omitempty"`
}

type TextItem struct {
	Text string `json:"text,omitempty"`
}

type ImageItem struct {
	Media       *CDNMedia `json:"media,omitempty"`
	ThumbMedia  *CDNMedia `json:"thumb_media,omitempty"`
	AESKey      string    `json:"aeskey,omitempty"`
	URL         string    `json:"url,omitempty"`
	MidSize     int64     `json:"mid_size,omitempty"`
	ThumbSize   int64     `json:"thumb_size,omitempty"`
	ThumbHeight int       `json:"thumb_height,omitempty"`
	ThumbWidth  int       `json:"thumb_width,omitempty"`
	HDSize      int64     `json:"hd_size,omitempty"`
}

type VoiceItem struct {
	Media         *CDNMedia `json:"media,omitempty"`
	EncodeType    int       `json:"encode_type,omitempty"`
	BitsPerSample int       `json:"bits_per_sample,omitempty"`
	SampleRate    int       `json:"sample_rate,omitempty"`
	PlayTime      int       `json:"playtime,omitempty"`
	Text          string    `json:"text,omitempty"`
}

type FileItem struct {
	Media    *CDNMedia `json:"media,omitempty"`
	FileName string    `json:"file_name,omitempty"`
	MD5      string    `json:"md5,omitempty"`
	Len      string    `json:"len,omitempty"`
}

type VideoItem struct {
	Media       *CDNMedia `json:"media,omitempty"`
	VideoSize   int64     `json:"video_size,omitempty"`
	PlayLength  int       `json:"play_length,omitempty"`
	VideoMD5    string    `json:"video_md5,omitempty"`
	ThumbMedia  *CDNMedia `json:"thumb_media,omitempty"`
	ThumbSize   int64     `json:"thumb_size,omitempty"`
	ThumbHeight int       `json:"thumb_height,omitempty"`
	ThumbWidth  int       `json:"thumb_width,omitempty"`
}

type RefMessage struct {
	MessageItem *MessageItem `json:"message_item,omitempty"`
	Title       string       `json:"title,omitempty"`
}

type MessageItem struct {
	Type         MessageItemType `json:"type,omitempty"`
	CreateTimeMs int64           `json:"create_time_ms,omitempty"`
	UpdateTimeMs int64           `json:"update_time_ms,omitempty"`
	IsCompleted  bool            `json:"is_completed,omitempty"`
	MsgID        string          `json:"msg_id,omitempty"`
	RefMsg       *RefMessage     `json:"ref_msg,omitempty"`
	TextItem     *TextItem       `json:"text_item,omitempty"`
	ImageItem    *ImageItem      `json:"image_item,omitempty"`
	VoiceItem    *VoiceItem      `json:"voice_item,omitempty"`
	FileItem     *FileItem       `json:"file_item,omitempty"`
	VideoItem    *VideoItem      `json:"video_item,omitempty"`
}

// WeixinMessage is the unified message structure from getUpdates.
type WeixinMessage struct {
	Seq          int64         `json:"seq,omitempty"`
	MessageID    int64         `json:"message_id,omitempty"`
	FromUserID   string        `json:"from_user_id,omitempty"`
	ToUserID     string        `json:"to_user_id,omitempty"`
	ClientID     string        `json:"client_id,omitempty"`
	CreateTimeMs int64         `json:"create_time_ms,omitempty"`
	UpdateTimeMs int64         `json:"update_time_ms,omitempty"`
	DeleteTimeMs int64         `json:"delete_time_ms,omitempty"`
	SessionID    string        `json:"session_id,omitempty"`
	GroupID      string        `json:"group_id,omitempty"`
	MessageType  MessageType   `json:"message_type,omitempty"`
	MessageState MessageState  `json:"message_state,omitempty"`
	ItemList     []MessageItem `json:"item_list,omitempty"`
	ContextToken string        `json:"context_token,omitempty"`
}

// --- GetUpdates ---

type GetUpdatesReq struct {
	GetUpdatesBuf string    `json:"get_updates_buf"`
	BaseInfo      *BaseInfo `json:"base_info,omitempty"`
}

type GetUpdatesResp struct {
	Ret                  int             `json:"ret,omitempty"`
	ErrCode              int             `json:"errcode,omitempty"`
	ErrMsg               string          `json:"errmsg,omitempty"`
	Msgs                 []WeixinMessage `json:"msgs,omitempty"`
	GetUpdatesBuf        string          `json:"get_updates_buf,omitempty"`
	LongPollingTimeoutMs int64           `json:"longpolling_timeout_ms,omitempty"`
}

// --- SendMessage ---

type SendMessageReq struct {
	Msg      *WeixinMessage `json:"msg,omitempty"`
	BaseInfo *BaseInfo      `json:"base_info,omitempty"`
}

// --- SendTyping ---

type SendTypingReq struct {
	ILinkUserID  string       `json:"ilink_user_id,omitempty"`
	TypingTicket string       `json:"typing_ticket,omitempty"`
	Status       TypingStatus `json:"status,omitempty"`
	BaseInfo     *BaseInfo    `json:"base_info,omitempty"`
}

// --- GetConfig ---

type GetConfigReq struct {
	ILinkUserID  string    `json:"ilink_user_id,omitempty"`
	ContextToken string    `json:"context_token,omitempty"`
	BaseInfo     *BaseInfo `json:"base_info,omitempty"`
}

type GetConfigResp struct {
	Ret          int    `json:"ret,omitempty"`
	ErrMsg       string `json:"errmsg,omitempty"`
	TypingTicket string `json:"typing_ticket,omitempty"`
}

// --- QR Login ---

type QRCodeResponse struct {
	QRCode           string `json:"qrcode"`
	QRCodeImgContent string `json:"qrcode_img_content"`
}

type QRStatusResponse struct {
	Status      string `json:"status"` // wait, scaned, confirmed, expired
	BotToken    string `json:"bot_token,omitempty"`
	ILinkBotID  string `json:"ilink_bot_id,omitempty"`
	BaseURL     string `json:"baseurl,omitempty"`
	ILinkUserID string `json:"ilink_user_id,omitempty"`
}

// LoginResult holds the final result of a QR login flow.
type LoginResult struct {
	Connected bool
	BotToken  string
	BotID     string
	BaseURL   string
	UserID    string
	Message   string
}
