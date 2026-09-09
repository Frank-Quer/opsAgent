package app

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestRecentSummariesAndFinalExclusion(t *testing.T) {
	var final cardState
	p := newProgress(context.Background(), time.Hour, func(_ context.Context, s cardState) error { final = s; return nil })
	for _, s := range []string{"一", "二", "三", "四", "四"} {
		p.report(s)
	}
	snapshot := p.snapshot("运行中", "", 0)
	if !reflect.DeepEqual(snapshot.Summaries, []string{"二", "三", "四"}) {
		t.Fatal(snapshot)
	}
	p.report("结论")
	p.start()
	if e := p.finish("已完成", "结论"); e != nil {
		t.Fatal(e)
	}
	if !reflect.DeepEqual(final.Summaries, []string{"二", "三", "四"}) || final.Result != "结论" {
		t.Fatal(final)
	}
	if !reflect.DeepEqual(snapshot.Summaries, []string{"二", "三", "四"}) {
		t.Fatal("snapshot mutated")
	}
}

func TestAnimationWithoutSummaries(t *testing.T) {
	frames := make(chan cardState, 10)
	p := newProgress(context.Background(), time.Millisecond, func(_ context.Context, s cardState) error { frames <- s; return nil })
	p.started = time.Now().Add(-65 * time.Second)
	p.start()
	for i := 0; i < 4; i++ {
		select {
		case s := <-frames:
			if s.Frame != (i+1)%3 || s.Elapsed < 65*time.Second {
				t.Fatal(s)
			}
		case <-time.After(time.Second):
			t.Fatal("missing heartbeat")
		}
	}
	if e := p.finish("已完成", "结论"); e != nil {
		t.Fatal(e)
	}
	for len(frames) > 0 {
		<-frames
	}
	select {
	case <-frames:
		t.Fatal("updated after completion")
	case <-time.After(5 * time.Millisecond):
	}
}

func TestHistoryCardStyles(t *testing.T) {
	for _, state := range []string{"运行中", "已完成", "失败", "超时"} {
		s := cardState{State: state, Summaries: []string{"旧一", "旧二", "新摘要"}, Elapsed: 65 * time.Second, Frame: 1}
		if state != "运行中" {
			s.Result = "结论"
		}
		data, e := cardContent(s)
		if e != nil {
			t.Fatal(e)
		}
		var card struct {
			Header struct {
				Title struct {
					Content string `json:"content"`
				} `json:"title"`
			} `json:"header"`
			Elements []struct {
				Tag string `json:"tag"`
			} `json:"elements"`
		}
		if e = json.Unmarshal([]byte(data), &card); e != nil {
			t.Fatal(e)
		}
		if !strings.Contains(card.Header.Title.Content, "1分05秒") {
			t.Fatal(card.Header)
		}
		for i := 0; i < 3; i++ {
			want := "note"
			if state == "运行中" && i == 2 {
				want = "div"
			}
			if card.Elements[i].Tag != want {
				t.Fatal(card.Elements)
			}
		}
		if state != "运行中" && card.Elements[3].Tag != "div" {
			t.Fatal("missing final result")
		}
	}
}
