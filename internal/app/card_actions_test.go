package app

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/larksuite/oapi-sdk-go/v3/event/dispatcher/callback"
	"opsagent/internal/codex"
)

func stopEvent(t *testing.T, user, card, chat string) *callback.CardActionTriggerEvent {
	t.Helper()
	data, _ := json.Marshal(map[string]any{
		"header": map[string]string{"app_id": "cli_test"},
		"event": map[string]any{
			"operator": map[string]string{"open_id": user},
			"action":   map[string]any{"tag": "button", "value": map[string]string{"action": "stop_task"}},
			"context":  map[string]string{"open_message_id": card, "open_chat_id": chat},
		},
	})
	var event callback.CardActionTriggerEvent
	if err := json.Unmarshal(data, &event); err != nil {
		t.Fatal(err)
	}
	return &event
}

func TestStopCardGuards(t *testing.T) {
	for _, tc := range []struct {
		name, user, card, chat string
		stop                   bool
	}{
		{"owner", "ou_owner", "om_card", "oc_chat", true},
		{"admin", "ou_admin", "om_card", "oc_chat", true},
		{"other", "ou_other", "om_card", "oc_chat", false},
		{"old card", "ou_owner", "om_old", "oc_chat", false},
		{"other chat", "ou_owner", "om_card", "oc_other", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			b := newBot(context.Background(), "ou_admin", nil, nil)
			b.appID = "cli_test"
			b.busy, b.activeUser, b.activeCard, b.activeChat, b.activeCancel = true, "ou_owner", "om_card", "oc_chat", cancel
			b.users = &userStore{members: map[string]map[string]bool{"cli_test": {"ou_owner": true, "ou_other": true}}}
			b.handleCardAction(context.Background(), stopEvent(t, tc.user, tc.card, tc.chat))
			if (ctx.Err() != nil) != tc.stop {
				t.Fatalf("cancelled=%v", ctx.Err())
			}
		})
	}
}

func TestStopTaskUpdatesCardAndReleasesBusy(t *testing.T) {
	b := testBot(context.Background(), "ou_me", nil, func(context.Context, string, string) error { return nil })
	started := make(chan struct{})
	b.run = func(ctx context.Context, _ codex.Request, _ func(string)) (codex.Result, error) {
		close(started)
		<-ctx.Done()
		return codex.Result{}, ctx.Err()
	}
	var final cardState
	b.updateCard = func(_ context.Context, _ string, state cardState) error { final = state; return nil }
	b.handle(message("om_task", "ou_me", "检查"))
	<-started
	event := stopEvent(t, "ou_me", "om_task", "oc_chat")
	event.EventV2Base.Header.AppID = b.appID
	response, err := b.handleCardAction(context.Background(), event)
	if err != nil || response.Toast.Type != "success" {
		t.Fatalf("%+v %v", response, err)
	}
	b.wg.Wait()
	if final.State != "已停止" || b.busy || b.activeCancel != nil || len(b.sessions) != 0 {
		t.Fatalf("state=%+v busy=%v", final, b.busy)
	}
	response, _ = b.handleCardAction(context.Background(), event)
	if response.Toast.Type == "success" {
		t.Fatal("finished task accepted")
	}
}

func TestStopButtonOnlyWhileRunning(t *testing.T) {
	for _, state := range []string{"运行中", "已完成", "已停止", "失败", "超时"} {
		content, err := cardContent(cardState{State: state})
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(content, `"stop_task"`) != (state == "运行中") {
			t.Fatalf("%s: %s", state, content)
		}
	}
}
