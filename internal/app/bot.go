package app

import (
	"context"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"

	"opsagent/internal/codex"
)

// bot.mu protects authorization, stores, sessions, deduplication and the active task.
// Register workers under mu before releasing it; stop closes admission before waiting.
type bot struct {
	users         *userStore
	activeUser    string
	activeCancel  context.CancelFunc
	activeRevoked bool
	bind          func(string) error
	help          func(context.Context, string, bool, string) error
	ctx           context.Context
	allowed       string
	botID         string
	appID         string
	workspaces    *workspaceStore
	models        func(context.Context, string) ([]codex.Model, error)
	prepare       func(context.Context, *larkim.EventMessage, string) (string, []string, func())
	started       int64
	timeout       time.Duration
	run           func(context.Context, codex.Request, func(string)) (codex.Result, error)
	reply         func(context.Context, string, string) error
	createCard    func(context.Context, string) (string, error)
	updateCard    func(context.Context, string, cardState) error
	mu            sync.Mutex
	wg            sync.WaitGroup
	closed, busy  bool
	seen          map[string]bool
	sessions      map[string]string
}

func newBot(ctx context.Context, allowed string, run func(context.Context, codex.Request, func(string)) (codex.Result, error), reply func(context.Context, string, string) error) *bot {
	return &bot{ctx: ctx, allowed: allowed, started: time.Now().UnixMilli(), timeout: 10 * time.Minute,
		run: run, reply: reply, seen: make(map[string]bool), sessions: make(map[string]string)}
}

func value(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func (b *bot) handle(e *larkim.P2MessageReceiveV1) {
	if e == nil || e.Event == nil || e.Event.Message == nil || e.Event.Sender == nil || e.Event.Sender.SenderId == nil {
		return
	}
	m, s := e.Event.Message, e.Event.Sender
	id, chat, user := value(m.MessageId), value(m.ChatId), value(s.SenderId.OpenId)
	if (value(m.ChatType) != "p2p" && value(m.ChatType) != "group") || value(s.SenderType) != "user" || !validID(id, "om_") || !validID(chat, "oc_") || !validID(user, "ou_") {
		return
	}
	ts, err := strconv.ParseInt(value(m.CreateTime), 10, 64)
	if err != nil || ts < b.started || ts > time.Now().Add(time.Minute).UnixMilli() {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed || b.seen[id] || b.ctx.Err() != nil {
		return
	}
	if value(m.ChatType) == "p2p" && value(m.MessageType) == "text" {
		text, _, err := messageText("text", value(m.Content), "")
		if err == nil && text == "/whoami" {
			b.seen[id] = true
			b.replyCommand(id, "你的用户 ID："+user)
			return
		}
	}
	if b.allowed == "" {
		if value(m.ChatType) != "p2p" || value(m.MessageType) != "text" {
			return
		}
		text, _, err := messageText("text", value(m.Content), "")
		if err != nil || text != "/bind" {
			return
		}
		b.seen[id] = true
		answer := "绑定失败，请检查本机 .env 配置后重试。"
		if err = b.bind(user); err == nil {
			b.allowed = user
			answer = "绑定成功。请使用 /project set /项目绝对路径 设置工作区，或 /help 查看指南。"
		}
		b.wg.Add(1)
		go func() { defer b.wg.Done(); b.send(id, answer) }()
		return
	}

	if user != b.allowed && !b.users.has(b.appID, user) {
		return
	}
	group := value(m.ChatType) == "group"
	if group {
		mentioned := false
		for _, mention := range m.Mentions {
			if mention != nil && mention.Id != nil && b.botID != "" && value(mention.Id.OpenId) == b.botID {
				mentioned = true
			}
		}
		if !mentioned {
			return
		}
	}
	text, _, parseErr := messageText(value(m.MessageType), value(m.Content), b.botID)
	if parseErr != nil {
		text = ""
	}
	for _, mention := range m.Mentions {
		if mention != nil && mention.Id != nil && value(mention.Id.OpenId) == b.botID && value(mention.Key) != "" {
			text = strings.ReplaceAll(text, value(mention.Key), "")
		}
	}
	text = strings.TrimSpace(text)
	if text == "" && parseErr == nil && validID(value(m.ParentId), "om_") {
		text = "分析被引用的消息"
	}

	b.seen[id] = true
	admin := user == b.allowed
	if text == "/whoami" {
		b.replyCommand(id, "你的用户 ID："+user)
		return
	}
	if text == "/users" || strings.HasPrefix(text, "/users ") {
		if !admin {
			b.replyCommand(id, "仅管理员可管理用户。")
		} else {
			b.replyCommand(id, b.userCommand(text, m.Mentions))
		}
		return
	}
	if !admin && text != "/help" && !group {
		b.replyCommand(id, "请在已配置工作区的群内 @ 机器人排查，私聊仅支持 /help 和 /whoami。")
		return
	}
	if !admin && strings.HasPrefix(text, "/") && text != "/help" && text != "/new" {
		b.replyCommand(id, "仅管理员可查看或修改项目、环境和模型设置。")
		return
	}
	if text == "/bind" && value(m.ChatType) == "p2p" {
		b.wg.Add(1)
		go func() { defer b.wg.Done(); b.send(id, "已绑定，无需重复操作。") }()
		return
	}
	if text == "/help" {
		b.wg.Add(1)
		go func() { defer b.wg.Done(); b.sendHelp(id, true, id) }()
		return
	}
	answer := ""
	dir := b.workspaces.get(b.appID, chat)
	environment := b.workspaces.environment(b.appID, chat)
	model := b.workspaces.model(b.appID, chat)
	modelCommand := text == "/models" || text == "/model" || strings.HasPrefix(text, "/model ")
	sessionKey := user + ":" + chat + "\x00" + dir
	switch {
	case text == "" || !utf8.ValidString(text) || len(text) > 32*1024:
		answer = "请补充问题或引用消息；支持文本、富文本和图片，文字最长 32 KB。"
	case b.busy:
		answer = "当前任务仍在执行，请收到结果后再发送。"
	case modelCommand:
		b.busy = true
	case text == "/project" || strings.HasPrefix(text, "/project "):
		answer = b.projectCommand(chat, text)
	case text == "/env" || strings.HasPrefix(text, "/env "):
		answer = b.environmentCommand(chat, text)
	case text == "/new":
		delete(b.sessions, sessionKey)
		answer = "已开始新对话。"
	case dir == "":
		answer = "尚未设置项目，请使用 /project set /绝对路径。"
	default:
		normalized, err := normalizeWorkspace(dir)
		if err != nil || normalized != dir {
			answer = "工作区目录已失效，请重新设置项目。"
		} else {
			b.busy = true
		}
	}
	b.wg.Add(1)
	if answer != "" {
		go func() { defer b.wg.Done(); b.send(id, answer) }()
		return
	}
	taskCtx, taskCancel := context.WithTimeout(b.ctx, b.timeout)
	b.activeUser = user
	b.activeCancel = taskCancel
	b.activeRevoked = false
	go b.executeTask(taskCtx, taskCancel, taskInput{
		message:    m,
		request:    codex.Request{Directory: dir, Session: b.sessions[sessionKey], Prompt: text, Model: model},
		sessionKey: sessionKey, environment: environment, modelCommand: modelCommand,
	})
}

func (b *bot) send(id, text string) bool {
	if err := sendTimeout(b.ctx, b.reply, id, text); err != nil {
		log.Printf("回复消息 %s 失败：%v", id, err)
		return false
	}
	return true
}

func (b *bot) stop() {
	b.mu.Lock()
	b.closed = true
	b.mu.Unlock()
	b.wg.Wait()
}

func splitReply(text string, limit int) []string {
	runes := []rune(text)
	var parts []string
	for len(runes) > limit {
		n := limit
		for i := limit - 1; i >= limit/2; i-- {
			if runes[i] == '\n' {
				n = i + 1
				break
			}
		}
		parts = append(parts, string(runes[:n]))
		runes = runes[n:]
	}
	if len(runes) > 0 {
		parts = append(parts, string(runes))
	}
	return parts
}
