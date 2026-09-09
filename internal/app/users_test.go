package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
)

func TestUserPersistence(t *testing.T) {
	p := filepath.Join(t.TempDir(), "users.json")
	s, e := loadUsers(p)
	if e != nil {
		t.Fatal(e)
	}
	if e = s.set("cli_test", "ou_member", true); e != nil {
		t.Fatal(e)
	}
	s, e = loadUsers(p)
	if e != nil || !s.has("cli_test", "ou_member") || s.has("cli_other", "ou_member") {
		t.Fatal("persistence/isolation")
	}
	s.path = filepath.Join(p, "bad")
	if s.set("cli_test", "ou_member", false) == nil || !s.has("cli_test", "ou_member") {
		t.Fatal("failed save mutated")
	}
	os.WriteFile(p, []byte("bad"), 0600)
	if _, e = loadUsers(p); e == nil {
		t.Fatal("accepted corruption")
	}
}
func TestMemberRevocation(t *testing.T) {
	started := make(chan struct{})
	b := testBot(context.Background(), "ou_me", func(ctx context.Context, s, p string, _ func(string), _ ...string) (string, string, error) {
		close(started)
		<-ctx.Done()
		return "thread-late", "private result", nil
	}, func(context.Context, string, string) error { return nil })
	b.users, _ = loadUsers(filepath.Join(t.TempDir(), "users.json"))
	b.users.set(b.appID, "ou_member", true)
	b.botID = "ou_bot"
	b.updateCard = func(_ context.Context, _ string, s cardState) error {
		if strings.Contains(s.Result, "private result") {
			t.Error("revoked result sent")
		}
		return nil
	}
	e := message("om_task", "ou_member", "检查")
	e.Event.Message.ChatType = ptr("group")
	e.Event.Message.Mentions = botMention()
	b.handle(e)
	<-started
	b.handle(message("om_remove", "ou_me", "/users remove ou_member"))
	b.wg.Wait()
	if b.users.has(b.appID, "ou_member") || len(b.sessions) > 0 {
		t.Fatal("revocation failed")
	}
}

func botMention() []*larkim.MentionEvent {
	return []*larkim.MentionEvent{{Key: ptr("@_user_1"), Id: &larkim.UserId{OpenId: ptr("ou_bot")}}}
}

func TestUserTargetsAndRoles(t *testing.T) {
	var replies []string
	b := testBot(context.Background(), "ou_me", nil, func(_ context.Context, _, text string) error { replies = append(replies, text); return nil })
	b.users, _ = loadUsers(filepath.Join(t.TempDir(), "users.json"))
	b.botID = "ou_bot"
	mentions := []*larkim.MentionEvent{{Key: ptr("@_user_2"), Id: &larkim.UserId{OpenId: ptr("ou_member")}, MentionedType: ptr("user")}}
	if !strings.Contains(b.userCommand("/users add @_user_2", mentions), "已添加") {
		t.Fatal("mention failed")
	}
	if !strings.Contains(b.userCommand("/users remove ou_me", nil), "不能") {
		t.Fatal("removed admin")
	}
	if !strings.Contains(b.userCommand("/users add ou_bot", nil), "请指定") {
		t.Fatal("added bot")
	}
	if !strings.Contains(b.userCommand("/users add ou_a ou_b", nil), "用法") {
		t.Fatal("multiple accepted")
	}
	old := b.workspaces.get(b.appID, "oc_chat")
	e := message("om_setting", "ou_member", "/project clear")
	e.Event.Message.ChatType = ptr("group")
	e.Event.Message.Mentions = botMention()
	b.handle(e)
	b.wg.Wait()
	if b.workspaces.get(b.appID, "oc_chat") != old {
		t.Fatal("member edited settings")
	}
	b.handle(message("om_private", "ou_member", "检查"))
	b.wg.Wait()
	b.handle(message("om_who", "ou_unknown", "/whoami"))
	b.wg.Wait()
	if !strings.Contains(strings.Join(replies, "\n"), "ou_unknown") {
		t.Fatal("whoami blocked")
	}
	b.sessions["ou_me:oc_chat\x00"+old] = "a"
	b.sessions["ou_member:oc_chat\x00"+old] = "b"
	b.sessions["ou_member:oc_other\x00"+old] = "c"
	b.clearChatSessions("oc_chat")
	if len(b.sessions) != 1 {
		t.Fatal("chat sessions not cleared")
	}
	b.users.path = filepath.Join(b.users.path, "bad")
	b.userCommand("/users remove ou_member", nil)
	if !b.users.has(b.appID, "ou_member") {
		t.Fatal("failed persistence revoked")
	}
}
