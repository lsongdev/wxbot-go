package wechatbot

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"time"
)

const (
	maxQRRefreshCount   = 3
	qrLongPollTimeout   = 35 * time.Second
	defaultLoginTimeout = 8 * time.Minute
)

// FetchQRCode requests a new login QR code from the API.
func (c *Client) FetchQRCode(ctx context.Context) (*QRCodeResponse, error) {
	base := ensureTrailingSlash(c.BaseURL)
	botType := c.BotType
	if botType == "" {
		botType = DefaultBotType
	}
	u, _ := url.JoinPath(base, "ilink/bot/get_bot_qrcode")
	u += "?bot_type=" + url.QueryEscape(botType)

	data, err := c.doGet(ctx, u, nil, 15*time.Second)
	if err != nil {
		return nil, fmt.Errorf("fetch QR code: %w", err)
	}

	var resp QRCodeResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("unmarshal QR response: %w", err)
	}
	return &resp, nil
}

// PollQRStatus polls the status of a QR code login.
func (c *Client) PollQRStatus(ctx context.Context, qrcode string) (*QRStatusResponse, error) {
	base := ensureTrailingSlash(c.BaseURL)
	u, _ := url.JoinPath(base, "ilink/bot/get_qrcode_status")
	u += "?qrcode=" + url.QueryEscape(qrcode)

	headers := map[string]string{
		"iLink-App-ClientVersion": "1",
	}

	data, err := c.doGet(ctx, u, headers, qrLongPollTimeout+5*time.Second)
	if err != nil {
		// Client-side timeout is normal for long-poll
		if ctx.Err() == nil {
			return &QRStatusResponse{Status: "wait"}, nil
		}
		return nil, err
	}

	var resp QRStatusResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("unmarshal QR status: %w", err)
	}
	return &resp, nil
}

// LoginWithQR performs the full QR code login flow:
// fetch QR -> poll status -> return credentials on confirm.
//
// On success, client.Token and client.BaseURL are updated automatically.
// Uses the handler from Config.
func (c *Client) LoginWithQR(ctx context.Context) (*LoginResult, error) {
	h := c.config.Handler
	if h == nil {
		h = &DefaultHandler{}
		c.config.Handler = h
	}
	h.SetClient(c)

	ctx, cancel := context.WithTimeout(ctx, defaultLoginTimeout)
	defer cancel()

	qr, err := c.FetchQRCode(ctx)
	if err != nil {
		return nil, err
	}
	h.OnQRCode(qr.QRCodeImgContent)

	scannedNotified := false
	refreshCount := 1
	currentQR := qr.QRCode

	for {
		select {
		case <-ctx.Done():
			result := &LoginResult{Message: "登录超时，请重试。"}
			h.OnLoginResult(result)
			return result, nil
		default:
		}

		status, err := c.PollQRStatus(ctx, currentQR)
		if err != nil {
			return nil, fmt.Errorf("poll QR status: %w", err)
		}

		switch status.Status {
		case "wait":
			// keep polling

		case "scaned":
			if !scannedNotified {
				scannedNotified = true
				h.OnScanned()
			}

		case "expired":
			refreshCount++
			if refreshCount > maxQRRefreshCount {
				result := &LoginResult{Message: "登录超时：二维码多次过期。"}
				h.OnLoginResult(result)
				return result, nil
			}
			h.OnExpired(refreshCount, maxQRRefreshCount)
			newQR, err := c.FetchQRCode(ctx)
			if err != nil {
				return nil, fmt.Errorf("refresh QR code: %w", err)
			}
			currentQR = newQR.QRCode
			scannedNotified = false
			h.OnQRCode(newQR.QRCodeImgContent)

		case "confirmed":
			if status.ILinkBotID == "" {
				result := &LoginResult{Message: "登录失败：服务器未返回 bot ID。"}
				h.OnLoginResult(result)
				return result, nil
			}
			c.Token = status.BotToken
			if c.config != nil {
				c.config.SetToken(status.BotToken)
			}
			if status.BaseURL != "" {
				c.BaseURL = status.BaseURL
			}
			result := &LoginResult{
				Connected: true,
				BotToken:  status.BotToken,
				BotID:     status.ILinkBotID,
				BaseURL:   status.BaseURL,
				UserID:    status.ILinkUserID,
				Message:   "与微信连接成功！",
			}
			h.OnLoginResult(result)
			return result, nil
		}

		time.Sleep(1 * time.Second)
	}
}
