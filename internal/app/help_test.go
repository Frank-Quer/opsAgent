package app

import (
	"context"
	"errors"
	"strings"
	"testing"

	larkevent "github.com/larksuite/oapi-sdk-go/v3/event"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
)

func joinEvent(id string) *larkim.P2ChatMemberBotAddedV1 {
	return &larkim.P2ChatMemberBotAddedV1{EventV2Base: &larkevent.EventV2Base{Header: &larkevent.EventHeader{EventID: id, AppID: "cli_test"}}, Event: &larkim.P2ChatMemberBotAddedV1Data{ChatId: ptr("oc_group")}}
}
func TestJoinHelpGuardsAndDedup(t *testing.T) {
	b := testBot(context.Background(), "ou_me", nil, nil)
	var ids []string
	b.help = func(_ context.Context, target string, reply bool, id string) error {
		if target != "oc_group" || reply {
			t.Error("wrong destination")
		}
		ids = append(ids, id)
		return errors.New("offline")
	}
	b.handleJoined(nil)
	bad := joinEvent("bad")
	bad.EventV2Base.Header.AppID = "cli_other"
	b.handleJoined(bad)
	b.handleJoined(joinEvent("first"))
	b.wg.Wait()
	b.handleJoined(joinEvent("first"))
	b.wg.Wait()
	b.handleJoined(joinEvent("second"))
	b.wg.Wait()
	if len(ids) != 2 || ids[0] == ids[1] {
		t.Fatal(ids)
	}
	b.allowed = ""
	b.handleJoined(joinEvent("third"))
	b.wg.Wait()
	if len(ids) != 2 {
		t.Fatal("unbound sent")
	}
}
func TestHelpWhileBusyWithoutWorkspace(t *testing.T) {
	b := testBot(context.Background(), "ou_me", nil, nil)
	b.workspaces.bindings = map[string]map[string]workspaceBinding{}
	b.busy = true
	calls := 0
	b.help = func(_ context.Context, target string, reply bool, id string) error {
		calls++
		if !reply || target != "om_help" {
			t.Error("wrong reply")
		}
		return nil
	}
	b.handle(message("om_other", "ou_other", "/help"))
	b.handle(message("om_help", "ou_me", "/help"))
	b.wg.Wait()
	if calls != 1 || !b.busy || len(b.sessions) != 0 {
		t.Fatal("help changed task")
	}
}
func TestHelpContent(t *testing.T) {
	content := helpCard()
	for _, s := range []string{"/new", "/help", "/whoami", "图片", "追问"} {
		if !strings.Contains(content, s) {
			t.Fatal(s)
		}
	}
	for _, s := range []string{"/project", "/env", "/model", "/users", "/bind", "管理员", "/Users/"} {
		if strings.Contains(content, s) {
			t.Fatalf("unexpected guide content: %s", s)
		}
	}
}
