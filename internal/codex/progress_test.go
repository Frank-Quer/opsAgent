package codex

import (
	"reflect"
	"strings"
	"testing"
)

func TestParseProgressFiltersEvents(t *testing.T) {
	input := ""
	for _, event := range []string{
		`{"type":"item.completed","item":{"type":"reasoning","text":"内部推理"}}`,
		`{"type":"item.completed","item":{"type":"command_execution","text":"原始命令"}}`,
		`{"type":"item.completed","item":{"type":"agent_message","text":"正在检查"}}`,
		`{"type":"item.completed","item":{"type":"agent_message","text":"最终结论"}}`,
		`{"type":"turn.completed"}`,
	} {
		input += event + "\n"
	}
	var summaries []string
	_, answer, err := parseCodex(strings.NewReader(input), "thread-1", func(s string) { summaries = append(summaries, s) })
	if err != nil || answer != "最终结论" || !reflect.DeepEqual(summaries, []string{"正在检查", "最终结论"}) {
		t.Fatalf("%q %q %v", answer, summaries, err)
	}
}
