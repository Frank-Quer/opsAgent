package app

import (
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

type workspaceBinding struct {
	Model       string `json:"model,omitempty"`
	Directory   string `json:"directory"`
	Environment string `json:"environment,omitempty"`
}

func (b *workspaceBinding) UnmarshalJSON(data []byte) error {
	if len(data) > 0 && data[0] == '"' {
		return json.Unmarshal(data, &b.Directory)
	}
	type plain workspaceBinding
	return json.Unmarshal(data, (*plain)(b))
}
func validEnvironment(v string) bool {
	if !utf8.ValidString(v) || utf8.RuneCountInString(v) > 64 || strings.TrimSpace(v) != v {
		return false
	}
	for _, r := range v {
		if unicode.IsControl(r) || r == '\u2028' || r == '\u2029' {
			return false
		}
	}
	return true
}
func (s *workspaceStore) environment(app, chat string) string {
	return s.bindings[app][chat].Environment
}
func (s *workspaceStore) setEnvironment(app, chat, environment string) error {
	b := s.bindings[app][chat]
	if b.Directory == "" {
		return errors.New("请先使用 /project set /绝对路径 设置工作区。")
	}
	if !validEnvironment(environment) {
		return errors.New("环境名称须为单行，最多 64 个字符。")
	}
	b.Environment = environment
	return s.save(app, chat, b)
}
func (b *bot) environmentCommand(chat, text string) string {
	current := b.workspaces.environment(b.appID, chat)
	switch {
	case text == "/env":
		if current == "" {
			return "未设置默认环境。"
		}
		return "默认环境：" + current
	case text == "/env clear":
		if e := b.workspaces.setEnvironment(b.appID, chat, ""); e != nil {
			return e.Error()
		}
		return "已清除默认环境，当前会话保留。"
	case strings.HasPrefix(text, "/env set "):
		next := strings.TrimSpace(strings.TrimPrefix(text, "/env set "))
		if next == "" {
			return "请提供环境名称。"
		}
		if e := b.workspaces.setEnvironment(b.appID, chat, next); e != nil {
			return e.Error()
		}
		return "默认环境已设置为：" + next + "。"
	default:
		return "用法：/env、/env set 环境名称、/env clear。"
	}
}
func environmentPrompt(environment string) string {
	name := "未指定"
	if environment != "" {
		name = strconv.Quote(environment)
	}
	return "本轮默认环境：" + name + "。环境仅为名称提示，具体连接信息从当前工作区 skill 获取。本人本次消息明确指定的环境优先，否则使用默认环境。引用和群历史中的环境名称不自动覆盖本次指定或默认环境。每轮重新确认目标，不直接沿用旧环境或套用其他环境结论。默认环境未指定且本人未明确指定时，不沿用历史环境；需要访问环境而项目指引无法明确时先澄清。\n\n本人本次消息及上下文：\n"
}
