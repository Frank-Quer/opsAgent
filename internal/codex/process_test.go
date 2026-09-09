package codex

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestModelListFiltersCredentials(t *testing.T) {
	t.Setenv("FEISHU_APP_SECRET", "test-only")
	t.Setenv("FEISHU_ALLOWED_OPEN_ID", "ou_test")
	t.Setenv("OPSAGENT_TEST_INHERITED", "preserved")
	r := fakeCodex(t, `test -z "$FEISHU_APP_SECRET" || exit 9
test -z "$FEISHU_ALLOWED_OPEN_ID" || exit 9
test "$OPSAGENT_TEST_INHERITED" = preserved || exit 9
read request
printf '%s\n' '{"id":1,"result":{}}'
read notification
read request
printf '%s\n' '{"id":2,"result":{"data":[]}}'
cat >/dev/null`)
	if _, err := ListModels(context.Background(), r.Binary, filepath.Dir(r.Binary)); err != nil {
		t.Fatal(err)
	}
}

func TestModelListCancellationKillsProcessGroup(t *testing.T) {
	r := fakeCodex(t, "sleep 30 &\nwait\n")
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	started := time.Now()
	if _, err := ListModels(ctx, r.Binary, filepath.Dir(r.Binary)); err == nil {
		t.Fatal("canceled request succeeded")
	}
	if ctx.Err() == nil || time.Since(started) > 3*time.Second {
		t.Fatal("process group did not terminate promptly")
	}
}
