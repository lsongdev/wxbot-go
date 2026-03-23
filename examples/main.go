package main

import (
	"context"
	"log"

	"github.com/lsongdev/wxbot-go/wxbot"
)

func main() {
	cfg := wxbot.LoadConfig("config.json")
	bot := wxbot.NewBot(cfg)
	ctx := context.Background()
	bot.Login(ctx)
	bot.Start(ctx, func(message *wxbot.ReplyMessage) {
		log.Println(message, message.Text())
		message.Typing(wxbot.Typing)
		message.ReplyText(message.Text())
		message.Typing(wxbot.CancelTyping)
		cfg.Save()
	})
}
