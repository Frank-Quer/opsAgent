package app

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"opsagent/internal/codex"
)

func TestModelStore(t *testing.T) {
	s, _ := loadWorkspaces(filepath.Join(t.TempDir(), "config.json"))
	s.set("cli_test", "oc_chat", t.TempDir())
	if e := s.setModel("cli_test", "oc_chat", "model-test"); e != nil {
		t.Fatal(e)
	}
	s, e := loadWorkspaces(s.path)
	if e != nil || s.model("cli_test", "oc_chat") != "model-test" {
		t.Fatal("not persisted")
	}
	if s.setModel("cli_test", "oc_chat", "bad\nmodel") == nil {
		t.Fatal("invalid accepted")
	}
}
func TestModelCommands(t *testing.T) {
	b := testBot(context.Background(), "ou_me", nil, func(context.Context, string, string) error { return nil })
	b.workspaces.path = filepath.Join(t.TempDir(), "config.json")
	b.models = func(context.Context, string) ([]codex.Model, error) {
		return []codex.Model{{Model: "model-a", Default: true}}, nil
	}
	dir := b.workspaces.get(b.appID, "oc_chat")
	b.sessions["test"] = "keep"
	if s := b.modelCommand(context.Background(), "oc_chat", "/model set model-a", dir); !strings.Contains(s, "已设置") {
		t.Fatal(s)
	}
	if b.workspaces.model(b.appID, "oc_chat") != "model-a" || b.workspaces.model(b.appID, "oc_a") != "" || b.sessions["test"] != "keep" {
		t.Fatal("scope or session changed")
	}
	b.modelCommand(context.Background(), "oc_chat", "/model set unknown", dir)
	if b.workspaces.model(b.appID, "oc_chat") != "model-a" {
		t.Fatal("invalid changed binding")
	}
	b.modelCommand(context.Background(), "oc_chat", "/model clear", dir)
	if b.workspaces.model(b.appID, "oc_chat") != "" {
		t.Fatal("clear failed")
	}
}
