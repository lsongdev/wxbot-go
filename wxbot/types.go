package wxbot

import (
	"encoding/json"
	"fmt"
	"os"
)

// Config holds the bot configuration.
type Config struct {
	path string `json:"-"`

	BaseURL    string `json:"base_url"`
	CDNBaseURL string `json:"cdn_base_url"`
	// Token is the bot token for authentication.
	// Empty for login-only usage; will be auto-updated after LoginWithQR.
	Token string `json:"token"`
	// internal: sync buffer for resuming after restart
	UpdatesBuf string `json:"updates_buf"`
}

// Save persists the config (token and buffer) to a file.
func (c *Config) Save() error {
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(c.path, b, 0600)
}

func LoadConfig(path string) (cfg *Config) {
	cfg = &Config{
		path: path,
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	err = json.Unmarshal(data, cfg)
	return
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

// --- CDN Download/Upload ---

// DownloadFileReq represents a CDN download request.
type DownloadFileReq struct {
	EncryptQueryParam string `json:"encrypt_query_param"`
}

// UploadFileResult holds the result after uploading a file to CDN.
type UploadFileResult struct {
	*CDNMedia
	FileSize int64 `json:"file_size"` // ciphertext size
	RawSize  int64 `json:"raw_size"`  // plaintext size
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
	MessageItemNone  MessageItemType = 0
	MessageItemText  MessageItemType = 1
	MessageItemImage MessageItemType = 2
	MessageItemVoice MessageItemType = 3
	MessageItemFile  MessageItemType = 4
	MessageItemVideo MessageItemType = 5
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
type Message struct {
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

// ExtractText is a helper to extract the first text body from a message's item list.
func (msg *Message) Text() string {
	for _, item := range msg.ItemList {
		if item.Type == MessageItemText && item.TextItem != nil {
			return item.TextItem.Text
		}
	}
	return ""
}

func (msg *Message) Image() *ImageItem {
	for _, item := range msg.ItemList {
		if item.Type == MessageItemImage && item.ImageItem != nil {
			return item.ImageItem
		}
	}
	return nil
}

// --- GetUpdates ---

type GetUpdatesReq struct {
	GetUpdatesBuf string    `json:"get_updates_buf"`
	BaseInfo      *BaseInfo `json:"base_info,omitempty"`
}

type WeChatBotError struct {
	ErrCode int    `json:"errcode,omitempty"`
	ErrMsg  string `json:"errmsg,omitempty"`
}

// Error implements [error].
func (b WeChatBotError) Error() string {
	return fmt.Sprintf("Error (%d): %s", b.ErrCode, b.ErrMsg)
}

type GetUpdatesResp struct {
	*WeChatBotError
	Messages      []Message `json:"msgs,omitempty"`
	SyncBuf       string    `json:"sync_buf"` // deprecated, use "get_updates_buf" instead.
	GetUpdatesBuf string    `json:"get_updates_buf,omitempty"`
}

// --- SendMessage ---

type SendMessageReq struct {
	Message  *Message  `json:"msg,omitempty"`
	BaseInfo *BaseInfo `json:"base_info,omitempty"`
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
	TypingTicket string `json:"typing_ticket,omitempty"`
}

// --- QR Login ---

type QRCodeResponse struct {
	Ret              int    `json:"ret"`
	QRCode           string `json:"qrcode"`
	QRCodeImgContent string `json:"qrcode_img_content"`
}

type QRStatusResponse struct {
	Ret         int    `json:"ret"`
	Status      string `json:"status"` // wait, scaned, confirmed, expired
	BotToken    string `json:"bot_token,omitempty"`
	ILinkBotID  string `json:"ilink_bot_id,omitempty"`
	ILinkUserID string `json:"ilink_user_id,omitempty"`
	BaseURL     string `json:"baseurl,omitempty"`
}
