package app

import (
	"strings"
	"testing"
)

func TestMessageTextInteractive(t *testing.T) {
	for _, tt := range []struct{ name, content, want string }{
		{"message API", `{"title":"限流告警","elements":[[{"tag":"text","text":"hyperliquid /info http 429: null"}]]}`, "限流告警\nhyperliquid /info http 429: null"},
		{"legacy", `{"header":{"title":{"tag":"plain_text","content":"限流告警"}},"elements":[{"tag":"div","text":{"tag":"lark_md","content":"http 429"},"fields":[{"text":{"tag":"plain_text","content":"生产环境"}}]},{"tag":"action","actions":[{"tag":"button","text":{"tag":"plain_text","content":"查看详情"},"value":{"text":"hidden"}}]}]}`, "限流告警\nhttp 429\n生产环境\n查看详情"},
		{"v2", `{"schema":"2.0","header":{"title":{"tag":"plain_text","content":"限流告警"}},"body":{"elements":[{"tag":"column_set","columns":[{"tag":"column","elements":[{"tag":"markdown","content":"http 429"}]}]}]}}`, "限流告警\nhttp 429"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, images, err := messageText("interactive", tt.content, "")
			if err != nil || got != tt.want || len(images) != 0 {
				t.Fatalf("text=%q images=%v err=%v", got, images, err)
			}
		})
	}
	for _, content := range []string{`{`, `null`, `[]`, `{}`, `{"elements":[]}`, `{"config":{"text":"hidden"}}`, strings.Repeat("x", 128*1024+1)} {
		if _, _, err := messageText("interactive", content, ""); err == nil {
			t.Fatalf("expected error for unreadable card: %.80s", content)
		}
	}
}
