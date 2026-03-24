# WeChat Bot Go SDK

[![Go Reference](https://pkg.go.dev/badge/github.com/lsongdev/wechatbot-go.svg)](https://pkg.go.dev/github.com/lsongdev/wechatbot-go)
[![Go Report Card](https://goreportcard.com/badge/github.com/lsongdev/wechatbot-go)](https://goreportcard.com/report/github.com/lsongdev/wechatbot-go)

A Go SDK for the WeChat iLink Bot API (微信 iLink Bot API).

## Features

- ✅ QR Code Login - Scan to authenticate your bot
- ✅ Message Long-Polling - Receive messages via `getupdates` API
- ✅ Text Messages - Send and receive text messages
- ✅ Media Support - Images, files, videos, and voice messages
- ✅ Typing Indicators - Show "typing..." status to users
- ✅ Auto Reconnection - Built-in retry logic with backoff
- ✅ Context Management - Automatic `context_token` handling

## Installation

```bash
go get github.com/lsongdev/wechatbot-go
```

## Quick Start

### Basic Echo Bot

```go
package main

import (
    "context"
    "log"

    "github.com/lsongdev/wechatbot-go/wechatbot"
)

func main() {
    // Load configuration from file
    cfg := wechatbot.LoadConfig("config.json")
    
    // Create bot instance
    bot := wechatbot.NewBot(cfg)
    ctx := context.Background()
    
    // Get QR code for login
    qrcode, err := bot.GetBotQRCode()
    if err != nil {
        log.Fatal(err)
    }
    log.Println("Scan QR Code:", qrcode.QRCodeImgContent)
    
    // Wait for user to scan and confirm
    resp, err := bot.WaitingForLogin(ctx, qrcode.QRCode)
    if err != nil {
        log.Fatal(err)
    }
    
    // Set the bot token after successful login
    bot.Token = resp.BotToken
    log.Println("Login successful:", resp.ILinkBotID)
    
    // Start message listener
    err = bot.Start(ctx, func(message *wechatbot.Message) {
        log.Printf("Message from %s: %s\n", message.FromUserID, message.Text())
        
        // Create reply helper
        response := bot.CreateReply(message)
        
        // Reply with text
        response.Typing(wechatbot.Typing)
        response.ReplyText("Echo: " + message.Text())
        response.Typing(wechatbot.CancelTyping)
        
        // Save config (token and sync buffer)
        cfg.Save()
    })
    
    log.Fatal(err)
}
```

### Configuration

Create a `config.json` file:

```json
{
  "base_url": "https://ilinkai.weixin.qq.com",
  "cdn_base_url": "https://novac2c.cdn.weixin.qq.com/c2c",
  "token": "",
  "updates_buf": ""
}
```

After first login, the `token` and `updates_buf` will be automatically saved.

## Advanced Usage

### Handling Images

```go
err = bot.Start(ctx, func(message *wechatbot.Message) {
    response := bot.CreateReply(message)
    
    // Check if message contains an image
    if img := message.Image(); img != nil {
        // Download the image
        imageData, err := bot.DownloadMedia(img.Media)
        if err != nil {
            log.Printf("Failed to download image: %v", err)
            return
        }
        
        // Reply with the same image
        fileName := fmt.Sprintf("image_%d.jpg", message.CreateTimeMs)
        _, err = response.ReplyImage(fileName, imageData)
        if err != nil {
            log.Printf("Failed to reply image: %v", err)
            return
        }
    } else {
        // Reply with text
        response.ReplyText(message.Text())
    }
})
```

### Sending Files

```go
// Send a file
fileData, _ := os.ReadFile("document.pdf")
_, err := bot.SendFile(contextToken, toUserID, "document.pdf", fileData)
if err != nil {
    log.Fatal(err)
}
```

### Sending Videos

```go
// Send a video with thumbnail
videoData, _ := os.ReadFile("video.mp4")
thumbData, _ := os.ReadFile("thumbnail.jpg")
_, err := bot.SendVideo(contextToken, toUserID, videoData, "video.mp4", thumbData)
if err != nil {
    log.Fatal(err)
}
```

### Typing Indicators

```go
response := bot.CreateReply(message)

// Show typing indicator
response.Typing(wechatbot.Typing)

// Simulate typing delay
time.Sleep(2 * time.Second)

// Send response
response.ReplyText("Your message has been processed.")

// Cancel typing indicator
response.Typing(wechatbot.CancelTyping)
```

## API Reference

### Core Types

#### `WeChatBot`

The main bot client.

```go
bot := wechatbot.NewBot(cfg)
```

#### `Config`

Bot configuration structure.

```go
type Config struct {
    BaseURL    string `json:"base_url"`
    CDNBaseURL string `json:"cdn_base_url"`
    Token      string `json:"token"`
    UpdatesBuf string `json:"updates_buf"`
}
```

### Message Types

#### `Message`

Represents an incoming message.

```go
type Message struct {
    FromUserID   string
    ToUserID     string
    CreateTimeMs int64
    MessageType  MessageType
    ItemList     []MessageItem
    ContextToken string
    // ... other fields
}
```

Helper methods:
- `Text()` - Extract text content
- `Image()` - Extract image item (returns `*ImageItem` or `nil`)

#### `MessageItem`

Individual message components (text, image, file, video, voice).

### Methods

#### Authentication

- `GetBotQRCode()` - Get QR code for login
- `GetQRCodeStatus(qrcode string)` - Check QR code status
- `WaitingForLogin(ctx context.Context, qrcode string)` - Wait for login confirmation
- `Login(ctx context.Context, force bool)` - Complete login flow

#### Message Handling

- `Start(ctx context.Context, onMessage MessageHandler)` - Start message listener (blocking)
- `GetUpdates(updateBuf string)` - Manual long-polling for messages
- `SendMessage(message *Message)` - Send a raw message
- `CreateReply(message *Message)` - Create reply helper

#### Reply Helpers

- `ReplyText(content string)` - Reply with text
- `ReplyImage(fileName string, imageData []byte)` - Reply with image
- `ReplyFile(fileName string, fileData []byte)` - Reply with file
- `ReplyVideo(fileName string, videoData []byte, thumbData []byte)` - Reply with video
- `Typing(status TypingStatus)` - Send typing indicator

#### Media Operations

- `DownloadMedia(media *CDNMedia)` - Download media from CDN
- `UploadFile(mediaType, toUserID, fileName string, data []byte, noThumb bool)` - Upload file to CDN
- `GetUploadURL(req *GetUploadURLReq)` - Get pre-signed upload URL

## Protocol Details

This SDK implements the WeChat iLink Bot API protocol. Key characteristics:

1. **QR Code Authentication** - Login via scanning QR code with WeChat
2. **Long-Polling** - Messages received via `getupdates` endpoint (not WebSocket)
3. **Context Token** - Every reply must include `context_token` from incoming message
4. **CDN Media** - Files uploaded/downloaded via separate CDN endpoint

For detailed protocol specifications, see [`docs/protocol-spec.md`](docs/protocol-spec.md).

## Error Handling

The SDK returns `WeChatBotError` for API errors:

```go
type WeChatBotError struct {
    ErrCode int    `json:"errcode"`
    ErrMsg  string `json:"errmsg"`
}
```

Common error codes:
- `-14` - Session expired, need to re-login
- `-2` - Invalid parameters

## Best Practices

### Persistence

Always save the config after receiving new messages to preserve:
- `token` - Bot authentication token
- `updates_buf` - Message sync buffer for resuming after restart

```go
cfg.Save()
```

### Context Management

- Never reuse `context_token` across different users
- Cache `typing_ticket` per user to avoid repeated `getconfig` calls
- Clear local state on `-14` (session expired) errors

### Media Upload

- Generate unique `filekey` for each upload (16-byte hex)
- Calculate `filesize` as AES-128-ECB encrypted size with PKCS7 padding
- Use `no_need_thumb: true` to skip thumbnail upload if not needed

## Examples

See the [`examples/`](examples) directory for complete working examples.

## License

MIT License

## Acknowledgments

- Based on the WeChat iLink Bot API protocol
- Inspired by official `@tencent-weixin/openclaw-weixin` implementation
