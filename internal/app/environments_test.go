package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnvironmentMigrationAndPersistence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "workspaces.json")
	dir := t.TempDir()
	old, _ := json.Marshal(map[string]map[string]string{"cli_test": {"oc_chat": dir}})
	os.WriteFile(path, old, 0600)
	s, e := loadWorkspaces(path)
	if e != nil {
		t.Fatal(e)
	}
	if e = s.setEnvironment("cli_test", "oc_chat", "测试环境"); e != nil {
		t.Fatal(e)
	}
	s, e = loadWorkspaces(path)
	if e != nil || s.environment("cli_test", "oc_chat") != "测试环境" || s.get("cli_test", "oc_chat") != dir {
		t.Fatal("migration failed")
	}
	if s.environment("cli_other", "oc_chat") != "" {
		t.Fatal("cross app")
	}
	s.path = filepath.Join(path, "bad")
	if s.setEnvironment("cli_test", "oc_chat", "") == nil || s.environment("cli_test", "oc_chat") != "测试环境" {
		t.Fatal("failed save mutated")
	}
	s.path = path
	s.set("cli_test", "oc_chat", dir)
	if s.environment("cli_test", "oc_chat") != "测试环境" {
		t.Fatal("same workspace cleared")
	}
	s.set("cli_test", "oc_chat", t.TempDir())
	if s.environment("cli_test", "oc_chat") != "" {
		t.Fatal("workspace switch retained env")
	}
	if s.setEnvironment("cli_test", "oc_missing", "prod") == nil {
		t.Fatal("unbound accepted")
	}
}
func TestEnvironmentValidation(t *testing.T) {
	for _, v := range []string{"\nprod", "bad\x00", strings.Repeat("字", 65)} {
		if validEnvironment(v) {
			t.Fatal("invalid accepted")
		}
	}
	if !validEnvironment("测试环境") || !validEnvironment("") {
		t.Fatal("valid rejected")
	}
}

func TestEnvironmentCommandsKeepSession(t *testing.T) {
	var prompts, sessions, replies []string
	b := testBot(context.Background(), "ou_me", func(_ context.Context, s, p string, _ func(string), _ ...string) (string, string, error) {
		sessions = append(sessions, s)
		prompts = append(prompts, p)
		return "thread-env", "结论", nil
	}, func(_ context.Context, _, text string) error { replies = append(replies, text); return nil })
	b.workspaces.path = filepath.Join(t.TempDir(), "workspaces.json")
	n := 0
	send := func(text string) { n++; b.handle(message(fmt.Sprintf("om_env%d", n), "ou_me", text)); b.wg.Wait() }
	send("检查")
	send("/env set 测试")
	send("检查生产环境")
	if sessions[1] != "thread-env" || !strings.Contains(prompts[1], `本轮默认环境："测试"`) || !strings.HasSuffix(prompts[1], "检查生产环境") {
		t.Fatal("lost session or override prompt")
	}
	send("/env")
	if replies[len(replies)-1] != "默认环境：测试" {
		t.Fatal(replies)
	}
	send("/new")
	if b.workspaces.environment(b.appID, "oc_chat") != "测试" {
		t.Fatal("new cleared env")
	}
	send("检查")
	if sessions[2] != "" {
		t.Fatal("new retained session")
	}
	if b.workspaces.environment(b.appID, "oc_a") != "" {
		t.Fatal("cross chat env")
	}
	send("/env clear")
	send("检查")
	if sessions[3] != "thread-env" || !strings.Contains(prompts[3], "本轮默认环境：未指定") {
		t.Fatal("clear lost session")
	}
	b.busy = true
	send("/env set prod")
	b.busy = false
	if b.workspaces.environment(b.appID, "oc_chat") != "" {
		t.Fatal("busy changed env")
	}
	send("/env set staging")
	send("/project clear")
	if b.workspaces.environment(b.appID, "oc_chat") != "" {
		t.Fatal("clear workspace retained env")
	}
	send("/env set prod")
	if b.workspaces.environment(b.appID, "oc_chat") != "" {
		t.Fatal("unbound accepted")
	}
}
