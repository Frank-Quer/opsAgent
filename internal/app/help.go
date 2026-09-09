package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log"
	"strings"
	"time"

	lark "github.com/larksuite/oapi-sdk-go/v3"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
)

func helpCard() string {
	text := `欢迎使用排查助手
获得授权后，可在已配置的群内 @ 机器人提问。

提问：@机器人 检查最近的报错
指定环境：@机器人 检查测试环境最近的报错
引用文字或图片后：@机器人 分析这个问题

排查卡片会实时展示摘要，完成后显示结论；收到结果后可继续 @ 追问。

/new 开始新对话
/help 再次查看指南
/whoami 查看自己的用户 ID

群内使用以上命令须 @ 机器人。私聊支持 /help 和 /whoami。`
	data, _ := json.Marshal(map[string]any{"config": map[string]bool{"wide_screen_mode": true}, "header": map[string]any{"title": map[string]string{"tag": "plain_text", "content": "排查助手 · 使用指南"}}, "elements": []any{map[string]any{"tag": "div", "text": map[string]string{"tag": "plain_text", "content": text}}}})
	return string(data)
}
func helpUUID(app, event string) string {
	sum := sha256.Sum256([]byte(app + ":" + event))
	return hex.EncodeToString(sum[:16])
}
func (b *bot) sendHelp(target string, reply bool, event string) {
	ctx, cancel := context.WithTimeout(b.ctx, 15*time.Second)
	defer cancel()
	if e := b.help(ctx, target, reply, helpUUID(b.appID, event)); e != nil {
		log.Print("帮助卡片发送失败")
	}
}
func (b *bot) handleJoined(e *larkim.P2ChatMemberBotAddedV1) {
	if e == nil || e.EventV2Base == nil || e.EventV2Base.Header == nil || e.Event == nil {
		return
	}
	h := e.EventV2Base.Header
	chat := value(e.Event.ChatId)
	if h.AppID != b.appID || !validID(chat, "oc_") || h.EventID == "" || len(h.EventID) > 128 || strings.ContainsAny(h.EventID, "\x00\r\n") {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	key := "join:" + h.EventID
	if b.allowed == "" || b.closed || b.ctx.Err() != nil || b.seen[key] {
		return
	}
	b.seen[key] = true
	b.wg.Add(1)
	go func() { defer b.wg.Done(); b.sendHelp(chat, false, h.EventID) }()
}
func setupHelp(b *bot, client *lark.Client) {
	b.help = func(ctx context.Context, target string, reply bool, id string) error {
		if reply {
			resp, e := client.Im.V1.Message.Reply(ctx, larkim.NewReplyMessageReqBuilder().MessageId(target).Body(larkim.NewReplyMessageReqBodyBuilder().MsgType("interactive").Content(helpCard()).Build()).Build())
			if e != nil || resp == nil || !resp.Success() {
				return errors.New("帮助卡片发送失败")
			}
		} else {
			resp, e := client.Im.V1.Message.Create(ctx, larkim.NewCreateMessageReqBuilder().ReceiveIdType("chat_id").Body(larkim.NewCreateMessageReqBodyBuilder().ReceiveId(target).MsgType("interactive").Content(helpCard()).Uuid(id).Build()).Build())
			if e != nil || resp == nil || !resp.Success() {
				return errors.New("入群帮助卡片发送失败")
			}
		}
		return nil
	}
}
