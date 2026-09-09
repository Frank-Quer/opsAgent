package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	lark "github.com/larksuite/oapi-sdk-go/v3"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	"github.com/larksuite/oapi-sdk-go/v3/event/dispatcher"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
	larkws "github.com/larksuite/oapi-sdk-go/v3/ws"

	"opsagent/internal/codex"
)

// Run dispatches the command line and starts the message service.
func Run(args []string) error {
	if len(args) > 0 && args[0] == "chat-context" {
		return runChatContext(args[1:])
	}
	if len(args) > 0 {
		return errors.New("用法：opsagent 或 opsagent chat-context <socket> <操作>")
	}
	cfg, err := loadConfig(os.Getenv)
	if err != nil {
		return err
	}
	appID, secret, allowed := cfg.appID, cfg.secret, cfg.allowed
	if _, err := exec.LookPath("codex"); err != nil {
		return errors.New("PATH 中找不到 codex")
	}
	configDir, err := os.UserConfigDir()
	if err != nil {
		return errors.New("无法定位用户配置目录")
	}
	workspaces, err := loadWorkspaces(filepath.Join(configDir, "opsagent", "workspaces.json"))
	if err != nil {
		return err
	}

	users, err := loadUsers(filepath.Join(configDir, "opsagent", "users.json"))
	if err != nil {
		return err
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	client := lark.NewClient(appID, secret, lark.WithLogLevel(larkcore.LogLevelError), lark.WithHttpClient(newFeishuHTTP()))
	reply := func(ctx context.Context, messageID, text string) error {
		body, err := json.Marshal(map[string]string{"text": text})
		if err != nil {
			return err
		}
		resp, err := client.Im.V1.Message.Reply(ctx, larkim.NewReplyMessageReqBuilder().
			MessageId(messageID).Body(larkim.NewReplyMessageReqBodyBuilder().
			MsgType("text").Content(string(body)).Build()).Build())
		if err != nil {
			return errors.New("飞书回复网络请求失败")
		}
		if resp == nil {
			return errors.New("飞书回复为空")
		}
		if !resp.Success() {
			return fmt.Errorf("飞书回复失败，code=%d", resp.Code)
		}
		return nil
	}
	bot := newBot(ctx, allowed, (&codex.Runner{Binary: "codex"}).Run, reply)
	bot.users = users
	bindingPath, err := filepath.Abs(".env")
	if err != nil {
		return errors.New("无法定位绑定配置")
	}
	bot.bind = func(id string) error { return saveBinding(bindingPath, id) }
	bot.models = func(ctx context.Context, dir string) ([]codex.Model, error) {
		return codex.ListModels(ctx, "codex", dir)
	}
	bot.appID = appID
	bot.workspaces = workspaces
	identity, err := botIdentity(ctx, client)
	if err != nil {
		return err
	}
	bot.botID = identity
	bot.prepare = prepareChat(client)
	setupCards(bot, client)
	setupHelp(bot, client)
	defer bot.stop()
	handler := dispatcher.NewEventDispatcher("", "").OnP2MessageReceiveV1(
		func(_ context.Context, event *larkim.P2MessageReceiveV1) error {
			bot.handle(event)
			return nil
		})
	handler.OnP2ChatMemberBotAddedV1(func(_ context.Context, e *larkim.P2ChatMemberBotAddedV1) error { bot.handleJoined(e); return nil })
	ws := larkws.NewClient(appID, secret, larkws.WithEventHandler(handler), larkws.WithLogLevel(larkcore.LogLevelError),
		larkws.WithOnReady(func() { log.Print("飞书长连接已就绪，可以保存事件订阅。") }),
		larkws.WithOnReconnecting(func() { log.Print("飞书连接中断，正在重连。") }),
		larkws.WithOnReconnected(func() { log.Print("飞书长连接已恢复。") }))
	if allowed == "" {
		log.Print("绑定模式：请本人私聊发送 /bind，首次绑定成功后立即生效。")
	} else {
		log.Print("对话模式：Codex 直接排查，单任务超时 10 分钟。")
	}
	log.Print("正在建立飞书长连接；请在后台保存长连接方式并订阅 im.message.receive_v1。")
	// SDK Start 在连接成功后持续运行；信号到达时取消任务并退出进程。
	done := make(chan error, 1)
	go func() { done <- ws.Start(ctx) }()
	select {
	case <-ctx.Done():
		return nil
	case err := <-done:
		cancel()
		if err != nil {
			return errors.New("飞书长连接启动失败，请检查凭据、机器人能力及网络")
		}
		return nil
	}
}

func validID(s, prefix string) bool {
	if !strings.HasPrefix(s, prefix) || len(s) <= len(prefix) || len(s) > 128 {
		return false
	}
	for _, c := range s {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '-') {
			return false
		}
	}
	return true
}

func sendTimeout(ctx context.Context, reply func(context.Context, string, string) error, id, text string) error {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	return reply(ctx, id, text)
}
