package app

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	lark "github.com/larksuite/oapi-sdk-go/v3"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
)

func botIdentity(ctx context.Context, client *lark.Client) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	r, e := client.Get(ctx, "/open-apis/bot/v3/info", nil, larkcore.AccessTokenTypeTenant)
	if e != nil || r == nil || r.StatusCode != 200 {
		return "", errors.New("无法获取机器人身份")
	}
	var v struct {
		Code int `json:"code"`
		Bot  struct {
			OpenID string `json:"open_id"`
		} `json:"bot"`
	}
	if json.Unmarshal(r.RawBody, &v) != nil || v.Code != 0 || !validID(v.Bot.OpenID, "ou_") {
		return "", errors.New("机器人身份无效")
	}
	return v.Bot.OpenID, nil
}

func messageText(kind, content, botID string) (string, []string, error) {
	if len(content) > 128*1024 {
		return "", nil, errors.New("消息过长")
	}
	var images []string
	switch kind {
	case "interactive":
		return cardText(content)
	case "text":
		var v struct {
			Text string `json:"text"`
		}
		if json.Unmarshal([]byte(content), &v) != nil {
			return "", nil, errors.New("消息格式无效")
		}
		return strings.TrimSpace(v.Text), nil, nil
	case "image":
		var v struct {
			Key string `json:"image_key"`
		}
		if json.Unmarshal([]byte(content), &v) != nil || v.Key == "" {
			return "", nil, errors.New("图片格式无效")
		}
		return "[图片]", []string{v.Key}, nil
	case "post":
		type node struct {
			Tag   string `json:"tag"`
			Text  string `json:"text"`
			User  string `json:"user_id"`
			Image string `json:"image_key"`
		}
		type post struct {
			Title   string   `json:"title"`
			Content [][]node `json:"content"`
		}
		var v post
		if json.Unmarshal([]byte(content), &v) != nil {
			return "", nil, errors.New("富文本格式无效")
		}
		if v.Content == nil {
			var locales map[string]json.RawMessage
			if json.Unmarshal([]byte(content), &locales) != nil {
				return "", nil, errors.New("富文本格式无效")
			}
			raw := locales["zh_cn"]
			if raw == nil {
				raw = locales["en_us"]
			}
			if raw == nil && len(locales) == 1 {
				for _, x := range locales {
					raw = x
				}
			}
			if raw == nil || json.Unmarshal(raw, &v) != nil {
				return "", nil, errors.New("富文本语言不支持")
			}
		}
		var out strings.Builder
		out.WriteString(v.Title)
		for _, row := range v.Content {
			out.WriteByte('\n')
			for _, n := range row {
				switch n.Tag {
				case "text", "a", "code_block":
					out.WriteString(n.Text)
				case "at":
					if n.User != botID {
						out.WriteString("[提及成员]")
					}
				case "img":
					if n.Image != "" {
						images = append(images, n.Image)
						out.WriteString("[图片]")
					}
				}
			}
		}
		return strings.TrimSpace(out.String()), images, nil
	default:
		return "[未解析的消息类型]", nil, errors.New("不支持该消息类型")
	}
}

func cardText(content string) (string, []string, error) {
	var card map[string]any
	if json.Unmarshal([]byte(content), &card) != nil || card == nil {
		return "", nil, errors.New("卡片格式无效")
	}
	var lines []string
	var walk func(any)
	walk = func(value any) {
		switch v := value.(type) {
		case string:
			if text := strings.TrimSpace(v); text != "" {
				lines = append(lines, text)
			}
		case []any:
			for _, child := range v {
				walk(child)
			}
		case map[string]any:
			// 只读取可见文本和布局字段，避免把回调 value、配置等当作正文。
			for _, key := range []string{"header", "title", "text"} {
				walk(v[key])
			}
			switch v["tag"] {
			case "plain_text", "lark_md", "markdown":
				if text, ok := v["content"].(string); ok {
					walk(text)
				}
			}
			for _, key := range []string{"body", "elements", "fields", "columns", "actions"} {
				walk(v[key])
			}
		}
	}
	walk(card)
	if len(lines) == 0 {
		return "", nil, errors.New("卡片没有可读取的文本")
	}
	return strings.Join(lines, "\n"), nil, nil
}
