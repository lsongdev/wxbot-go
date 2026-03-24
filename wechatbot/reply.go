package wechatbot

type ReplyMessage struct {
	*WeChatBot
	*Message
}

func (c *WeChatBot) CreateReply(message *Message) *ReplyMessage {
	return &ReplyMessage{
		WeChatBot: c,
		Message:   message,
	}
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

// ReplyImage uploads and sends an image message as a reply.
func (r *ReplyMessage) ReplyImage(fileName string, imageData []byte) (*SendMessageResp, error) {
	return r.SendImage(r.ContextToken, r.FromUserID, fileName, imageData)
}

// ReplyFile uploads and sends a file message as a reply.
func (r *ReplyMessage) ReplyFile(fileName string, fileData []byte) (*SendMessageResp, error) {
	return r.SendFile(r.ContextToken, r.FromUserID, fileName, fileData)
}

// ReplyVideo uploads and sends a video message as a reply.
func (r *ReplyMessage) ReplyVideo(fileName string, videoData []byte, thumbData []byte) (*SendMessageResp, error) {
	return r.SendVideo(r.ContextToken, r.FromUserID, videoData, fileName, thumbData)
}
