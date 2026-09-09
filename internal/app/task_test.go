package app

import (
	"context"
	"reflect"
	"strings"
	"testing"

	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
	"opsagent/internal/codex"
)

func TestTaskPassesRequestAndResumes(t *testing.T) {
	b := testBot(context.Background(), "ou_me", nil, func(context.Context, string, string) error { return nil })
	binding := b.workspaces.bindings[b.appID]["oc_chat"]
	binding.Environment, binding.Model = "测试环境", "model-test"
	b.workspaces.bindings[b.appID]["oc_chat"] = binding
	cleaned := 0
	b.prepare = func(_ context.Context, _ *larkim.EventMessage, prompt string) (string, []string, func()) {
		return prompt + " 上下文", []string{"/tmp/example.png"}, func() { cleaned++ }
	}
	var requests []codex.Request
	b.run = func(_ context.Context, req codex.Request, _ func(string)) (codex.Result, error) {
		requests = append(requests, req)
		return codex.Result{Session: "thread-test", Output: "结论"}, nil
	}
	for _, id := range []string{"om_first", "om_second"} {
		b.handle(message(id, "ou_me", "检查"))
		b.wg.Wait()
	}
	if len(requests) != 2 || cleaned != 2 {
		t.Fatalf("requests=%d cleanup=%d", len(requests), cleaned)
	}
	if requests[0].Session != "" || requests[1].Session != "thread-test" {
		t.Fatal("session not resumed")
	}
	for _, req := range requests {
		if req.Directory != binding.Directory || req.Model != binding.Model || !reflect.DeepEqual(req.Images, []string{"/tmp/example.png"}) || !strings.Contains(req.Prompt, "测试环境") || !strings.HasSuffix(req.Prompt, "检查 上下文") {
			t.Fatalf("invalid request: %+v", req)
		}
	}
}
