package main

import (
	"context"
	"fmt"
	"log"

	"github.com/lsongdev/wechatbot-go/wechatbot"
)

func main() {
	cfg := wechatbot.LoadConfig("config.json")
	bot := wechatbot.NewBot(cfg)
	ctx := context.Background()
	qrcode, err := bot.GetBotQRCode()
	log.Println("WeChat QRCode:", qrcode.QRCodeImgContent)
	resp, err := bot.WaitingForLogin(ctx, qrcode.QRCode)
	log.Println("WeChat Login:", resp.ILinkBotID)
	bot.Token = resp.BotToken
	err = bot.Start(ctx, func(message *wechatbot.Message) {
		log.Printf("Message from %s: %s\n", message.FromUserID, message.Text())
		response := bot.CreateReply(message)
		// // Check if message contains an image
		if img := message.Image(); img != nil {
			log.Println("Received image, downloading...")
			imageData, err := bot.DownloadMedia(img.Media)
			if err != nil {
				log.Printf("failed to download image: %v", err)
				return
			}
			fileName := fmt.Sprintf("%s_%d.jpg", message.FromUserID, message.CreateTimeMs)
			_, err = response.ReplyImage(fileName, imageData)
			if err != nil {
				log.Printf("failed to reply image: %v", err)
				return
			}
			log.Println("Image sent successfully")
		} else {
			// Echo text messages
			response.Typing(wechatbot.Typing)
			response.ReplyText(message.Text())
			response.Typing(wechatbot.CancelTyping)
		}
		cfg.Save()
	})
	log.Fatal(err)
}
