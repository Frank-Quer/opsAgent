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
	if e := s.setModel("cli_test", "oc_chat", "model-test", "high"); e != nil {
		t.Fatal(e)
	}
	s, e := loadWorkspaces(s.path)
	if e != nil || s.model("cli_test", "oc_chat") != "model-test" || s.bindings["cli_test"]["oc_chat"].ReasoningEffort != "high" {
		t.Fatal("not persisted")
	}
	if s.setModel("cli_test", "oc_chat", "bad\nmodel", "") == nil {
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

func TestReasoningEffortCommands(t *testing.T) {
	b := testBot(context.Background(), "ou_me", nil, func(context.Context, string, string) error { return nil })
	b.workspaces.path = filepath.Join(t.TempDir(), "config.json")
	b.models = func(context.Context, string) ([]codex.Model, error) {
		return []codex.Model{{Model: "model-a", SupportedReasoningEfforts: []codex.ReasoningEffort{{ReasoningEffort: "high"}}}}, nil
	}
	command := func(text string) string {
		return b.modelCommand(context.Background(), "oc_chat", text, b.workspaces.get(b.appID, "oc_chat"))
	}
	if got := command("/models"); !strings.HasPrefix(got, "当前模型：跟随本机 Codex 配置\n思考强度：跟随本机 Codex 配置\n") || !strings.Contains(got, "思考强度：high") {
		t.Fatal(got)
	}
	if got := command("/model set model-a high"); !strings.Contains(got, "已设置") {
		t.Fatal(got)
	}
	if got := command("/model"); !strings.Contains(got, "思考强度：high") {
		t.Fatal(got)
	}
	if got := command("/models"); !strings.HasPrefix(got, "当前模型：model-a\n思考强度：high\n") {
		t.Fatal(got)
	}
	for _, text := range []string{"/model set model-a low", "/model set model-a invalid", "/model set model-a high extra"} {
		if got := command(text); strings.Contains(got, "已设置") {
			t.Fatal(got)
		}
		if b.workspaces.bindings[b.appID]["oc_chat"].ReasoningEffort != "high" {
			t.Fatal("invalid command changed effort")
		}
	}
	command("/model set model-a")
	if b.workspaces.bindings[b.appID]["oc_chat"].ReasoningEffort != "" {
		t.Fatal("omitted effort not reset")
	}
	command("/model set model-a high")
	command("/model clear")
	if binding := b.workspaces.bindings[b.appID]["oc_chat"]; binding.Model != "" || binding.ReasoningEffort != "" {
		t.Fatal("clear failed")
	}
}
