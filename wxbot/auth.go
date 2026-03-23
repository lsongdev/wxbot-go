package wxbot

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"
)

const (
	maxQRRefreshCount   = 3
	qrLongPollTimeout   = 35 * time.Second
	defaultLoginTimeout = 8 * time.Minute
)

// FetchQRCode requests a new login QR code from the API.
//
//	{
//	    "qrcode": "b73e84f617fe08e311355ef30255039b",
//	    "qrcode_img_content": "https://liteapp.weixin.qq.com/q/7GiQu1?qrcode=b73e84f617fe08e311355ef30255039b&bot_type=3",
//	    "ret": 0
//	}
func (c *Client) GetBotQRCode() (resp *QRCodeResponse, err error) {
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
func (c *Client) GetQRCodeStatus(qrcode string) (resp *QRStatusResponse, err error) {
	path := fmt.Sprintf("/ilink/bot/get_qrcode_status?qrcode=%s", qrcode)
	// headers := map[string]string{
	// 	"iLink-App-ClientVersion": "1",
	// }
	err = c.request(http.MethodGet, path, nil, &resp)
	return
}

func (c *Client) SetToken(token string) {
	c.config.Token = token
}

func (c *Client) WaitingBotToken(ctx context.Context, qrcode string) (token string, err error) {
	var resp *QRStatusResponse
	for {
		select {
		case <-ctx.Done():
			return token, ctx.Err()
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
		case "confirmed":
			token = resp.BotToken
			return
		case "expired":
			err = fmt.Errorf("qrcode %s", resp.Status)
			return
		}
	}
}

func (c *Client) Login(ctx context.Context) {
	if c.config.Token == "" {
		qrcode, err := c.GetBotQRCode()
		if err != nil {
			log.Fatal(err)
		}
		log.Println("QRCode:", qrcode.QRCodeImgContent)
		botToken, err := c.WaitingBotToken(ctx, qrcode.QRCode)
		if err != nil {
			log.Fatal(err)
		}
		c.config.Token = botToken
	}
}
