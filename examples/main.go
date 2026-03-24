package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/lsongdev/wxbot-go/wxbot"
)

func main() {
	cfg := wxbot.LoadConfig("config.json")
	bot := wxbot.NewBot(cfg)
	ctx := context.Background()
	bot.Login(ctx, false)

	// Create images directory
	imagesDir := "images"
	if err := os.MkdirAll(imagesDir, 0755); err != nil {
		log.Fatalf("failed to create images directory: %v", err)
	}

	err := bot.Start(ctx, func(message *wxbot.ReplyMessage) {
		log.Printf("Message from %s: %s\n", message.FromUserID, message.Text())

		// Check if message contains an image
		if img := message.Image(); img != nil {
			log.Println("Received image, downloading...")

			// Download image using CDN
			imageData, err := bot.DownloadFile(img.Media.EncryptQueryParam, img.Media.AESKey)
			if err != nil {
				log.Printf("failed to download image: %v", err)
				return
			}

			// Save image to local file
			fileName := fmt.Sprintf("%s_%d.jpg", message.FromUserID, message.CreateTimeMs)
			filePath := filepath.Join(imagesDir, fileName)
			if err := os.WriteFile(filePath, imageData, 0644); err != nil {
				log.Printf("failed to save image: %v", err)
				return
			}

			log.Printf("Image saved to: %s", filePath)

			// Reply with the same image
			log.Println("Sending image back...")
			log.Printf("Image size: %d bytes, fileName: %s", len(imageData), fileName)
			_, err = message.ReplyImage(imageData, fileName)
			if err != nil {
				log.Printf("failed to reply image: %v", err)
				return
			}
			log.Println("Image sent successfully")
			return
		}

		// Echo text messages
		message.Typing(wxbot.Typing)
		message.ReplyText(message.Text())
		message.Typing(wxbot.CancelTyping)

		cfg.Save()
	})
	log.Fatal(err)
}
