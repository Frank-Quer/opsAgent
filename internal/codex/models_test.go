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
	for _, session := range []string{"", "thread-1"} {
		if _, e := r.Run(context.Background(), Request{Directory: filepath.Dir(r.Binary), Session: session, Prompt: "check", Model: "model-test"}, nil); e != nil {
			t.Fatal(e)
		}
		data, _ := os.ReadFile(filepath.Join(filepath.Dir(r.Binary), "args"))
		if !strings.Contains(string(data), "--model\nmodel-test\n") {
			t.Fatal("missing model")
		}
	}
}

func TestModelListProtocol(t *testing.T) {
	r := fakeCodex(t, `read first
printf '%s\n' '{"id":1,"result":{}}'
read notification
read request
printf '%s\n' '{"id":2,"result":{"data":[{"model":"model-a","isDefault":true}],"nextCursor":"next"}}'
read request
printf '%s\n' '{"id":3,"result":{"data":[{"model":"model-b"}],"nextCursor":null}}'
cat >/dev/null`)
	models, e := ListModels(context.Background(), r.Binary, filepath.Dir(r.Binary))
	if e != nil || len(models) != 2 || models[1].Model != "model-b" {
		t.Fatalf("%v %v", models, e)
	}
}
