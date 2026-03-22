package main

import (
	"context"
	"fmt"
	"log"
	"os/signal"
	"syscall"

	wechatbot "github.com/lsongdev/wechatbot-go/wechatbot"
)

const configPath = "config.json"

// BotHandler embeds DefaultHandler and overrides only the methods we need.
type BotHandler struct {
	wechatbot.DefaultHandler
}

// OnQRCode is called when a QR code URL is available.
func (h *BotHandler) OnQRCode(url string) {
	fmt.Printf("\n请用微信扫描二维码:\n%s\n\n", url)
}

// OnScanned is called once when the user has scanned the QR code.
func (h *BotHandler) OnScanned() {
	fmt.Println("已扫码，请在微信上确认...")
}

// OnExpired is called when the QR code expires.
func (h *BotHandler) OnExpired(attempt, max int) {
	fmt.Printf("二维码已过期，正在刷新 (%d/%d)...\n", attempt, max)
}

// OnLoginResult is called when login completes.
func (h *BotHandler) OnLoginResult(result *wechatbot.LoginResult) {
	if result.Connected {
		fmt.Printf("登录成功! BotID=%s UserID=%s\n", result.BotID, result.UserID)
	} else {
		fmt.Printf("登录未完成：%s\n", result.Message)
	}
}

// OnMessage is called for each inbound message.
func (h *BotHandler) OnMessage(ctx context.Context, msg wechatbot.WeixinMessage) {
	text := wechatbot.ExtractText(&msg)
	if text == "" {
		return
	}

	fmt.Printf("[来自 %s]: %s\n", msg.FromUserID, text)

	// Echo reply
	_, err := h.Client().SendText(ctx, msg.FromUserID, text, msg.ContextToken)
	if err != nil {
		log.Printf("回复失败：%v", err)
	}
}

// OnBufUpdate is called when the sync buffer updates.
func (h *BotHandler) OnBufUpdate(buf string) {
	// First, call the parent implementation to store in config
	h.DefaultHandler.OnBufUpdate(buf)
	// Then persist config to file
	if cfg := h.Client().Config(); cfg != nil {
		_ = cfg.Save(configPath)
	}
}

func main() {
	// Create handler and config
	handler := &BotHandler{}
	cfg := &wechatbot.Config{
		Handler: handler,
	}

	// Load config from file (token and buffer)
	if err := cfg.Load(configPath); err != nil {
		log.Printf("加载配置失败：%v", err)
	}
	if cfg.GetToken() != "" {
		fmt.Println("已加载保存的 Token")
	}
	if cfg.GetBuf() != "" {
		fmt.Println("从上次中断处恢复...")
	}

	client := wechatbot.NewClient(cfg)

	// --- Step 1: QR Login (skip if already have token) ---
	if cfg.GetToken() == "" {
		fmt.Println("正在获取登录二维码...")
		result, err := client.LoginWithQR(context.Background())
		if err != nil {
			log.Fatalf("登录失败：%v", err)
		}
		if !result.Connected {
			log.Fatalf("登录未完成：%s", result.Message)
		}
		// Save config after successful login
		if err := cfg.Save(configPath); err != nil {
			log.Printf("保存配置失败：%v", err)
		}
	}

	// Token is auto-saved to config
	fmt.Printf("Token: %s\n", cfg.GetToken())

	// --- Step 2: Start monitoring ---
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	fmt.Println("开始监听消息... (Ctrl+C 退出)")
	if err := client.Start(ctx); err != nil && err != context.Canceled {
		log.Fatalf("监听异常：%v", err)
	}
	fmt.Println("已退出")
}
