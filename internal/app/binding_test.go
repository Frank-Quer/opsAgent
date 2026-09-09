package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestBindFirstWinsAndFailure(t *testing.T) {
	b := testBot(context.Background(), "", nil, func(context.Context, string, string) error { return nil })
	saved := ""
	b.bind = func(id string) error { saved = id; return nil }
	group := message("om_group", "ou_a", "/bind")
	group.Event.Message.ChatType = ptr("group")
	b.handle(group)
	if saved != "" {
		t.Fatal("group bound")
	}
	var wg sync.WaitGroup
	for _, id := range []string{"a", "b"} {
		wg.Add(1)
		go func(id string) { defer wg.Done(); b.handle(message("om_"+id, "ou_"+id, "/bind")) }(id)
	}
	wg.Wait()
	b.wg.Wait()
	if saved == "" || b.allowed != saved {
		t.Fatal("not bound")
	}
	first := saved
	b.handle(message("om_again", "ou_other", "/bind"))
	b.wg.Wait()
	if saved != first {
		t.Fatal("overwritten")
	}
	b.allowed = ""
	b.bind = func(string) error { return errors.New("disk failure") }
	b.handle(message("om_failed", "ou_c", "/bind"))
	b.wg.Wait()
	if b.allowed != "" {
		t.Fatal("bound despite save failure")
	}
}
func TestSaveBinding(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	original := "export FEISHU_APP_SECRET='test-only'\nexport FEISHU_ALLOWED_OPEN_ID=''\n"
	os.WriteFile(path, []byte(original), 0600)
	if e := saveBinding(path, "ou_me"); e != nil {
		t.Fatal(e)
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "FEISHU_APP_SECRET='test-only'") || !strings.Contains(string(data), "FEISHU_ALLOWED_OPEN_ID='ou_me'") {
		t.Fatal("wrong config")
	}
	if e := saveBinding(path, "ou_other"); e == nil {
		t.Fatal("overwrote existing binding")
	}
	if e := saveBinding(path, "ou_me"); e != nil {
		t.Fatal(e)
	}
	st, _ := os.Stat(path)
	if st.Mode().Perm() != 0600 {
		t.Fatal("permissions")
	}
	if e := saveBinding(filepath.Join(path, "bad"), "ou_me"); e == nil {
		t.Fatal("invalid path")
	}
}
