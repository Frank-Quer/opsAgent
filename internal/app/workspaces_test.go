package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"

	"opsagent/internal/codex"
)

func TestWorkspacePersistenceAndFailure(t *testing.T) {
	file := filepath.Join(t.TempDir(), "config", "workspaces.json")
	s, e := loadWorkspaces(file)
	if e != nil {
		t.Fatal(e)
	}
	dir := filepath.Join(t.TempDir(), "project with spaces")
	os.Mkdir(dir, 0700)
	normalized, e := normalizeWorkspace(dir)
	if e != nil {
		t.Fatal(e)
	}
	if e = s.set("cli_test", "oc_one", normalized); e != nil {
		t.Fatal(e)
	}
	loaded, e := loadWorkspaces(file)
	if e != nil || loaded.get("cli_test", "oc_one") != normalized || loaded.get("cli_other", "oc_one") != "" {
		t.Fatal("lost isolation or persistence")
	}
	st, _ := os.Stat(file)
	if st.Mode().Perm() != 0600 {
		t.Fatal("file permissions")
	}
	st, _ = os.Stat(filepath.Dir(file))
	if st.Mode().Perm() != 0700 {
		t.Fatal("directory permissions")
	}
	s.path = filepath.Join(file, "impossible")
	if s.set("cli_test", "oc_one", "") == nil || s.get("cli_test", "oc_one") != normalized {
		t.Fatal("failed save changed memory")
	}
	if loaded.set("cli_test", "oc_one", "") != nil {
		t.Fatal("clear failed")
	}
	loaded, e = loadWorkspaces(file)
	if e != nil || loaded.get("cli_test", "oc_one") != "" {
		t.Fatal("clear not persisted")
	}
	os.WriteFile(file, []byte("broken"), 0600)
	if _, e = loadWorkspaces(file); e == nil {
		t.Fatal("accepted damaged config")
	}
}

func TestWorkspaceValidation(t *testing.T) {
	for _, p := range []string{"relative", "/no-such-workspace", "\x00"} {
		if _, e := normalizeWorkspace(p); e == nil {
			t.Fatal("accepted invalid path")
		}
	}
	dir := t.TempDir()
	link := filepath.Join(t.TempDir(), "alias")
	os.Symlink(dir, link)
	a, _ := normalizeWorkspace(dir)
	b, e := normalizeWorkspace(link)
	if e != nil || a != b {
		t.Fatal("symlink not resolved")
	}
}

func TestProjectCommandsAndSessions(t *testing.T) {
	a, _ := normalizeWorkspace(t.TempDir())
	other, _ := normalizeWorkspace(t.TempDir())
	store, e := loadWorkspaces(filepath.Join(t.TempDir(), "workspaces.json"))
	if e != nil {
		t.Fatal(e)
	}
	var sessions, dirs, replies []string
	prepared := 0
	b := testBot(context.Background(), "ou_me", func(context.Context, string, string, func(string), ...string) (string, string, error) {
		t.Fatal("wrong runner")
		return "", "", nil
	}, func(_ context.Context, _, text string) error { replies = append(replies, text); return nil })
	b.workspaces = store
	b.prepare = func(_ context.Context, _ *larkim.EventMessage, p string) (string, []string, func()) {
		prepared++
		return p, nil, func() {}
	}
	b.run = func(_ context.Context, req codex.Request, _ func(string)) (codex.Result, error) {
		dirs = append(dirs, req.Directory)
		sessions = append(sessions, req.Session)
		return codex.Result{Session: "thread-1", Output: "结论"}, nil
	}
	n := 0
	send := func(text string) { n++; b.handle(message(fmt.Sprintf("om_%d", n), "ou_me", text)); b.wg.Wait() }
	send("检查")
	if prepared != 0 || len(sessions) != 0 {
		t.Fatal("unbound ran or prepared")
	}
	send("/project set " + a)
	send("检查")
	send("/project set " + a)
	send("继续")
	send("/new")
	send("检查")
	send("/project set " + other)
	send("检查")
	send("/project set " + a)
	send("检查")
	if strings.Join(sessions, ",") != ",thread-1,,," {
		t.Fatalf("sessions=%q", sessions)
	}
	if dirs[3] != other || dirs[4] != a {
		t.Fatal("wrong workspace")
	}
	send("/project")
	if !strings.Contains(replies[len(replies)-1], filepath.Base(a)) || strings.Contains(replies[len(replies)-1], a) {
		t.Fatal("full path disclosed")
	}
	send("/project clear")
	before := prepared
	send("检查")
	if prepared != before || store.get("cli_test", "oc_chat") != "" {
		t.Fatal("clear ineffective")
	}
	send("/project set " + a)
	oldPath := store.path
	store.path = filepath.Join(oldPath, "bad")
	b.sessions["ou_me:oc_chat\x00"+a] = "existing"
	send("/project set " + other)
	if store.get("cli_test", "oc_chat") != a || b.sessions["ou_me:oc_chat\x00"+a] != "existing" {
		t.Fatal("failed save reset session")
	}
	store.path = oldPath
	os.RemoveAll(a)
	before = prepared
	send("检查")
	if prepared != before {
		t.Fatal("invalid directory prepared task")
	}
}

func TestProjectBusyDoesNotChangeBinding(t *testing.T) {
	b := testBot(context.Background(), "ou_me", nil, func(context.Context, string, string) error { return nil })
	b.busy = true
	old := b.workspaces.get(b.appID, "oc_chat")
	b.handle(message("om_busy_project", "ou_me", "/project clear"))
	b.wg.Wait()
	if b.workspaces.get(b.appID, "oc_chat") != old {
		t.Fatal("changed binding while busy")
	}
}
