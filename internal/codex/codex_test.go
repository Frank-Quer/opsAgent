package codex

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func fakeCodex(t *testing.T, script string) *Runner {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "codex")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+script), 0700); err != nil {
		t.Fatal(err)
	}
	return &Runner{Binary: path}
}

func TestRunnerNewAndResume(t *testing.T) {
	t.Setenv("FEISHU_APP_SECRET", "test-only-secret")
	r := fakeCodex(t, `test -z "$FEISHU_APP_SECRET" || exit 9
printf '%s\n' "$@" > args
cat > prompt
printf '%s\n' '{"type":"thread.started","thread_id":"thread-123"}' '{"type":"item.completed","item":{"type":"command_execution","text":"private log"}}' '{"type":"item.completed","item":{"type":"agent_message","text":"你好"}}' '{"type":"turn.completed"}'
`)
	for _, session := range []string{"", "thread-123"} {
		result, err := r.Run(context.Background(), Request{Directory: filepath.Dir(r.Binary), Session: session, Prompt: "$(touch injected); `echo hi`"}, nil)
		if err != nil || result.Session != "thread-123" || result.Output != "你好" {
			t.Fatalf("%+v %v", result, err)
		}
		args, _ := os.ReadFile(filepath.Join(filepath.Dir(r.Binary), "args"))
		want := []string{"-a", "never", "exec", "--sandbox", "danger-full-access"}
		if session != "" {
			want = append(want, "resume")
		}
		want = append(want, "--json", "--skip-git-repo-check")
		if session != "" {
			want = append(want, session)
		}
		want = append(want, "-")
		if !reflect.DeepEqual(strings.Split(strings.TrimSpace(string(args)), "\n"), want) {
			t.Fatalf("unexpected args=%s", args)
		}
		if session != "" && !strings.Contains(string(args), "resume\n--json\n--skip-git-repo-check\nthread-123\n-\n") {
			t.Fatalf("resume args=%s", args)
		}
		prompt, _ := os.ReadFile(filepath.Join(filepath.Dir(r.Binary), "prompt"))
		for _, rule := range []string{
			"shell、SSH", "禁止在飞书回复中输出", "密钥", "服务器 IP", "不输出命令", "1～3 句", "证据不足",
			"每轮排查（包括续聊）", "开展具体检查前", ".docs/troubleshooting.md", "用 rg 检索", "rg 不可用时用 grep",
			"结合本次环境和证据复核", "不作为额外操作授权", "每轮最终回复前", "已完全查明且有证据支持",
			"值得复用的新经验", "仅告警恢复或没有新增价值时不写入", "同类经验优先合并或修正", "不重复追加",
			"未经执行验证的修复建议不能写成有效处理方法", "只维护当前工作区的指南", "不自动提交 Git",
		} {
			if !strings.Contains(string(prompt), rule) {
				t.Fatalf("missing rule %q", rule)
			}
		}
		if !strings.Contains(string(prompt), "$(touch injected)") {
			t.Fatal("prompt altered")
		}
		if _, err := os.Stat(filepath.Join(filepath.Dir(r.Binary), "injected")); !os.IsNotExist(err) {
			t.Fatal("shell injection")
		}
	}
}

func TestRunnerTimeoutKillsProcessGroup(t *testing.T) {
	r := fakeCodex(t, "sleep 30 &\nwait\n")
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := r.Run(ctx, Request{Directory: filepath.Dir(r.Binary), Prompt: "test"}, nil)
	if !errors.Is(err, context.DeadlineExceeded) || time.Since(start) > 3*time.Second {
		t.Fatalf("%v duration=%v", err, time.Since(start))
	}
}

func TestParseCodexFailures(t *testing.T) {
	for _, input := range []string{
		"not json\n", "{\"type\":\"turn.failed\"}\n",
		"{\"type\":\"turn.completed\"}\n",
		"{\"type\":\"item.completed\",\"item\":{\"type\":\"agent_message\",\"text\":\"unfinished\"}}\n",
	} {
		if _, _, err := parseCodex(strings.NewReader(input), "thread-1", nil); err == nil {
			t.Fatalf("accepted %q", input)
		}
	}
}

func TestParseOnlyFinalAnswer(t *testing.T) {
	input := "{\"type\":\"item.completed\",\"item\":{\"type\":\"agent_message\",\"text\":\"正在处理\"}}\n" +
		"{\"type\":\"item.completed\",\"item\":{\"type\":\"agent_message\",\"text\":\"最终回答\"}}\n" +
		"{\"type\":\"turn.completed\"}\n"
	_, answer, err := parseCodex(strings.NewReader(input), "thread-1", nil)
	if err != nil || answer != "最终回答" {
		t.Fatalf("%q %v", answer, err)
	}
}

func TestRunnerNonzeroExit(t *testing.T) {
	r := fakeCodex(t, `printf '%s\n' '{"type":"item.completed","item":{"type":"agent_message","text":"answer"}}' '{"type":"turn.completed"}'
exit 7
`)
	if _, err := r.Run(context.Background(), Request{Directory: filepath.Dir(r.Binary), Session: "thread-1", Prompt: "test"}, nil); err == nil || !strings.Contains(err.Error(), "exit=7") {
		t.Fatalf("%v", err)
	}
}

func TestRunnerImageArguments(t *testing.T) {
	r := fakeCodex(t, `printf '%s\n' "$@" > args
cat >/dev/null
printf '%s\n' '{"type":"thread.started","thread_id":"thread-image"}' '{"type":"item.completed","item":{"type":"agent_message","text":"结论"}}' '{"type":"turn.completed"}'
`)
	for _, session := range []string{"", "thread-image"} {
		_, err := r.Run(context.Background(), Request{Directory: filepath.Dir(r.Binary), Session: session, Prompt: "分析", Images: []string{"/tmp/image with spaces.png"}}, nil)
		if err != nil {
			t.Fatal(err)
		}
		data, _ := os.ReadFile(filepath.Join(filepath.Dir(r.Binary), "args"))
		if !strings.Contains(string(data), "--image\n/tmp/image with spaces.png\n") {
			t.Fatal("missing image argument")
		}
	}
}

func TestRunnerUsesRequestedWorkspace(t *testing.T) {
	r := fakeCodex(t, `pwd > actual-cwd
cat >/dev/null
printf '%s\n' '{"type":"thread.started","thread_id":"thread-dir"}' '{"type":"item.completed","item":{"type":"agent_message","text":"结论"}}' '{"type":"turn.completed"}'
`)
	for _, session := range []string{"", "thread-dir"} {
		dir, _ := filepath.EvalSymlinks(t.TempDir())
		if _, err := r.Run(context.Background(), Request{Directory: dir, Session: session, Prompt: "检查"}, nil); err != nil {
			t.Fatal(err)
		}
		got, err := os.ReadFile(filepath.Join(dir, "actual-cwd"))
		if err != nil || strings.TrimSpace(string(got)) != dir {
			t.Fatalf("cwd=%q err=%v", got, err)
		}
	}
}
