package codex

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestModelArgs(t *testing.T) {
	r := fakeCodex(t, `printf '%s\n' "$@" > args
cat >/dev/null
printf '%s\n' '{"type":"thread.started","thread_id":"thread-1"}' '{"type":"item.completed","item":{"type":"agent_message","text":"OK"}}' '{"type":"turn.completed"}'`)
	for _, effort := range []string{"", "high"} {
		for _, session := range []string{"", "thread-1"} {
			if _, e := r.Run(context.Background(), Request{Directory: filepath.Dir(r.Binary), Session: session, Prompt: "check", Model: "model-test", ReasoningEffort: effort}, nil); e != nil {
				t.Fatal(e)
			}
			data, _ := os.ReadFile(filepath.Join(filepath.Dir(r.Binary), "args"))
			if !strings.Contains(string(data), "--model\nmodel-test\n") {
				t.Fatal("missing model")
			}
			if strings.Contains(string(data), "model_reasoning_effort=") != (effort != "") {
				t.Fatalf("unexpected args: %s", data)
			}
			if effort != "" && !strings.Contains(string(data), "-c\nmodel_reasoning_effort=\"high\"\n") {
				t.Fatalf("missing effort: %s", data)
			}
		}
	}
}

func TestModelListProtocol(t *testing.T) {
	r := fakeCodex(t, `read first
printf '%s\n' '{"id":1,"result":{}}'
read notification
read request
printf '%s\n' '{"id":2,"result":{"data":[{"model":"model-a","isDefault":true,"supportedReasoningEfforts":[{"reasoningEffort":"high","description":"Thorough"}]}],"nextCursor":"next"}}'
read request
printf '%s\n' '{"id":3,"result":{"data":[{"model":"model-b"}],"nextCursor":null}}'
cat >/dev/null`)
	models, e := ListModels(context.Background(), r.Binary, filepath.Dir(r.Binary))
	if e != nil || len(models) != 2 || models[1].Model != "model-b" || len(models[0].SupportedReasoningEfforts) != 1 || models[0].SupportedReasoningEfforts[0].ReasoningEffort != "high" {
		t.Fatalf("%v %v", models, e)
	}
}
