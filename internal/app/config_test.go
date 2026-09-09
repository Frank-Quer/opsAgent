package app

import (
	"strings"
	"testing"
)

func TestLoadConfig(t *testing.T) {
	for _, tc := range []struct {
		name, app, secret, user string
		valid                   bool
	}{
		{"binding", "cli_test", "test-only", "", true},
		{"trim", " cli_test ", " test-only ", " ou_admin ", true},
		{"missing secret", "cli_test", "", "", false},
		{"invalid app", "cli_", "test-only", "", false},
		{"invalid user", "cli_test", "test-only", "other", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := map[string]string{"FEISHU_APP_ID": tc.app, "FEISHU_APP_SECRET": tc.secret, "FEISHU_ALLOWED_OPEN_ID": tc.user}
			cfg, err := loadConfig(func(key string) string { return env[key] })
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%v err=%v", tc.valid, err)
			}
			if tc.valid && (cfg.appID != strings.TrimSpace(tc.app) || cfg.secret != strings.TrimSpace(tc.secret) || cfg.allowed != strings.TrimSpace(tc.user)) {
				t.Fatal("config changed")
			}
		})
	}
}

func TestRunRejectsArgumentsBeforeStartup(t *testing.T) {
	for _, args := range [][]string{{"unknown"}, {"chat-context"}, {"chat-context", "unused", "unknown"}, {"chat-context", "unused", "image", "om_one", "invalid"}} {
		if Run(args) == nil {
			t.Fatalf("accepted %q", args)
		}
	}
	t.Setenv("FEISHU_APP_ID", "")
	if err := Run(nil); err == nil || !strings.Contains(err.Error(), "FEISHU_APP_ID") {
		t.Fatalf("unexpected startup result: %v", err)
	}
}
