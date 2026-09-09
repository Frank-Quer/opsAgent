package app

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"

	"opsagent/internal/codex"
)

func ptr(s string) *string { return &s }

func message(id, user, text string) *larkim.P2MessageReceiveV1 {
	body, _ := json.Marshal(map[string]string{"text": text})
	return &larkim.P2MessageReceiveV1{Event: &larkim.P2MessageReceiveV1Data{
		Sender:  &larkim.EventSender{SenderType: ptr("user"), SenderId: &larkim.UserId{OpenId: ptr(user)}},
		Message: &larkim.EventMessage{MessageId: ptr(id), ChatId: ptr("oc_chat"), ChatType: ptr("p2p"), MessageType: ptr("text"), Content: ptr(string(body)), CreateTime: ptr(strconv.FormatInt(time.Now().UnixMilli(), 10))},
	}}
}

func TestMessageGuardsAndSessions(t *testing.T) {
	var mu sync.Mutex
	var sessions, replies []string
	run := func(_ context.Context, session, prompt string, _ func(string), _ ...string) (string, string, error) {
		mu.Lock()
		defer mu.Unlock()
		sessions = append(sessions, session)
		return "thread-1", "回答", nil
	}
	reply := func(_ context.Context, _, text string) error {
		mu.Lock()
		defer mu.Unlock()
		replies = append(replies, text)
		return nil
	}
	b := testBot(context.Background(), "ou_me", run, reply)
	b.handle(nil)
	b.handle(message("om_stranger", "ou_other", "不要执行"))
	old := message("om_old", "ou_me", "旧消息")
	old.Event.Message.CreateTime = ptr(strconv.FormatInt(b.started-1, 10))
	b.handle(old)
	group := message("om_group", "ou_me", "群消息")
	group.Event.Message.ChatType = ptr("group")
	b.handle(group)
	first := message("om_first", "ou_me", "第一问")
	b.handle(first)
	b.wg.Wait()
	b.handle(first)
	b.wg.Wait()
	b.handle(message("om_second", "ou_me", "继续"))
	b.wg.Wait()
	b.handle(message("om_new", "ou_me", "/new"))
	b.wg.Wait()
	b.handle(message("om_third", "ou_me", "重新问"))
	b.wg.Wait()
	if strings.Join(sessions, ",") != ",thread-1," {
		t.Fatalf("sessions=%q", sessions)
	}
	if len(replies) != 7 {
		t.Fatalf("replies=%q", replies)
	}
	b.stop()
	b.handle(message("om_closed", "ou_me", "已关闭"))
	if len(sessions) != 3 {
		t.Fatal("executed after stop")
	}
}

func TestUnboundDoesNotExecuteOrReply(t *testing.T) {
	b := testBot(context.Background(), "", func(context.Context, string, string, func(string), ...string) (string, string, error) {
		t.Error("executed")
		return "", "", nil
	}, func(context.Context, string, string) error { t.Error("replied"); return nil })
	b.handle(message("om_bind", "ou_me", "绑定"))
	b.stop()
}

func TestBusyAndTimeout(t *testing.T) {
	started := make(chan struct{})
	var mu sync.Mutex
	var replies []string
	b := testBot(context.Background(), "ou_me", func(ctx context.Context, _, _ string, _ func(string), _ ...string) (string, string, error) {
		close(started)
		<-ctx.Done()
		return "", "", ctx.Err()
	}, func(_ context.Context, _, text string) error {
		mu.Lock()
		defer mu.Unlock()
		replies = append(replies, text)
		return nil
	})
	b.timeout = 100 * time.Millisecond
	b.handle(message("om_task", "ou_me", "任务"))
	<-started
	b.handle(message("om_busy", "ou_me", "/new"))
	b.wg.Wait()
	all := strings.Join(replies, "\n")
	if !strings.Contains(all, "仍在执行") || !strings.Contains(all, "已停止") || b.busy {
		t.Fatalf("%s busy=%v", all, b.busy)
	}
	if len(b.sessions) != 0 {
		t.Fatal("saved failed session")
	}
}

func TestReceiptFailureDoesNotRun(t *testing.T) {
	b := testBot(context.Background(), "ou_me", func(context.Context, string, string, func(string), ...string) (string, string, error) {
		t.Error("executed")
		return "", "", nil
	}, func(context.Context, string, string) error { return errors.New("offline") })
	b.handle(message("om_receipt", "ou_me", "任务"))
	b.stop()
}

func TestSplitReply(t *testing.T) {
	text := strings.Repeat("中文🙂", 2100) + "\n\n" + strings.Repeat("结尾", 1000)
	parts := splitReply(text, 3000)
	if strings.Join(parts, "") != text {
		t.Fatal("lost text")
	}
	for _, p := range parts {
		if len([]rune(p)) > 3000 {
			t.Fatal("oversized chunk")
		}
	}
}

func testBot(ctx context.Context, allowed string, run func(context.Context, string, string, func(string), ...string) (string, string, error), reply func(context.Context, string, string) error) *bot {
	b := newBot(ctx, allowed, func(ctx context.Context, req codex.Request, progress func(string)) (codex.Result, error) {
		next, output, err := run(ctx, req.Session, req.Prompt, progress, req.Images...)
		return codex.Result{Session: next, Output: output}, err
	}, reply)
	dir, _ := os.Getwd()
	dir, _ = normalizeWorkspace(dir)
	b.appID = "cli_test"
	b.workspaces = &workspaceStore{bindings: map[string]map[string]workspaceBinding{"cli_test": {"oc_chat": {Directory: dir}, "oc_a": {Directory: dir}, "oc_b": {Directory: dir}}}}
	b.createCard = func(ctx context.Context, id string) (string, error) { return id, reply(ctx, id, "运行中") }
	b.updateCard = func(ctx context.Context, id string, state cardState) error { return reply(ctx, id, state.Result) }
	return b
}

func TestCardFinalFailureFallsBack(t *testing.T) {
	runs := 0
	var replies []string
	b := testBot(context.Background(), "ou_me", func(context.Context, string, string, func(string), ...string) (string, string, error) {
		runs++
		return "thread-1", "结论", nil
	}, func(_ context.Context, _, text string) error { replies = append(replies, text); return nil })
	b.updateCard = func(context.Context, string, cardState) error { return errors.New("offline") }
	b.handle(message("om_task", "ou_me", "排查"))
	b.wg.Wait()
	if runs != 1 || strings.Join(replies, ",") != "运行中,结论" {
		t.Fatalf("runs=%d replies=%q", runs, replies)
	}
}
