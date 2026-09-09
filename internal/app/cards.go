package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	lark "github.com/larksuite/oapi-sdk-go/v3"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
)

func cardContent(s cardState) (string, error) {
	// 纯文本元素不解析模型输出中的链接、图片或交互标签。
	title := "排查 · " + s.State
	if s.State == "运行中" {
		title = "排查中 " + strings.Repeat("·", s.Frame%3+1)
	}
	seconds := int(s.Elapsed.Seconds())
	title += fmt.Sprintf(" · %d分%02d秒", seconds/60, seconds%60)
	elements := []any{}
	for i, text := range s.Summaries {
		plain := map[string]string{"tag": "plain_text", "content": text}
		if s.State == "运行中" && i == len(s.Summaries)-1 {
			elements = append(elements, map[string]any{"tag": "div", "text": plain})
		} else {
			elements = append(elements, map[string]any{"tag": "note", "elements": []any{plain}})
		}
	}
	if s.Result != "" {
		elements = append(elements, map[string]any{"tag": "div", "text": map[string]string{"tag": "plain_text", "content": s.Result}})
	} else if len(elements) == 0 {
		elements = append(elements, map[string]any{"tag": "div", "text": map[string]string{"tag": "plain_text", "content": "已收到，正在开始排查。"}})
	}
	if s.State == "运行中" {
		elements = append(elements, map[string]any{"tag": "action", "actions": []any{
			map[string]any{"tag": "button", "type": "danger", "text": map[string]string{"tag": "plain_text", "content": "终止"}, "value": map[string]string{"action": "stop_task"}},
		}})
	}
	body, err := json.Marshal(map[string]any{
		"config":   map[string]any{"wide_screen_mode": true, "update_multi": true},
		"header":   map[string]any{"title": map[string]string{"tag": "plain_text", "content": title}},
		"elements": elements,
	})
	if err != nil {
		return "", err
	}
	if len(body) > 28000 {
		return "", errors.New("卡片内容超过限制")
	}
	return string(body), nil
}

func setupCards(b *bot, client *lark.Client) {
	b.createCard = func(ctx context.Context, id string) (string, error) {
		content, err := cardContent(cardState{State: "运行中"})
		if err != nil {
			return "", err
		}
		resp, err := client.Im.V1.Message.Reply(ctx, larkim.NewReplyMessageReqBuilder().MessageId(id).Body(larkim.NewReplyMessageReqBodyBuilder().MsgType("interactive").Content(content).Build()).Build())
		if err != nil || resp == nil || !resp.Success() || resp.Data == nil || resp.Data.MessageId == nil || *resp.Data.MessageId == "" {
			return "", errors.New("排查卡片发送失败")
		}
		return *resp.Data.MessageId, nil
	}
	b.updateCard = func(ctx context.Context, id string, state cardState) error {
		content, err := cardContent(state)
		if err != nil {
			return err
		}
		resp, err := client.Im.V1.Message.Patch(ctx, larkim.NewPatchMessageReqBuilder().MessageId(id).Body(larkim.NewPatchMessageReqBodyBuilder().Content(content).Build()).Build())
		if err != nil || resp == nil || !resp.Success() {
			return errors.New("排查卡片更新失败")
		}
		return nil
	}
}
