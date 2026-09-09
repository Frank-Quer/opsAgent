package app

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestProgressCoalescesAndFinishesLast(t *testing.T) {
	var updates []string
	sent := make(chan struct{}, 1)
	p := newProgress(context.Background(), time.Millisecond, func(_ context.Context, s cardState) error {
		state, text := s.State, s.Result
		if state == "运行中" && len(s.Summaries) > 0 {
			text = s.Summaries[len(s.Summaries)-1]
		}
		updates = append(updates, state+":"+text)
		sent <- struct{}{}
		return nil
	})
	p.report("旧摘要")
	p.report("最新摘要")
	p.start()
	<-sent
	if err := p.finish("已完成", "结论"); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(updates, []string{"运行中:最新摘要", "已完成:结论"}) {
		t.Fatal(updates)
	}
}

func TestProgressFailureStopsUpdates(t *testing.T) {
	calls := 0
	sent := make(chan struct{}, 1)
	p := newProgress(context.Background(), time.Millisecond, func(context.Context, cardState) error {
		calls++
		sent <- struct{}{}
		return errors.New("offline")
	})
	p.report("摘要")
	p.start()
	<-sent
	p.report("后续摘要")
	if p.finish("已完成", "结论") == nil || calls != 1 {
		t.Fatalf("calls=%d", calls)
	}
}

func TestProgressFinishDiscardsPending(t *testing.T) {
	var updates []string
	p := newProgress(context.Background(), time.Hour, func(_ context.Context, s cardState) error {
		state, text := s.State, s.Result
		if state == "运行中" && len(s.Summaries) > 0 {
			text = s.Summaries[len(s.Summaries)-1]
		}
		updates = append(updates, state+":"+text)
		return nil
	})
	p.start()
	p.report("过期摘要")
	if err := p.finish("已完成", "结论"); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(updates, []string{"已完成:结论"}) {
		t.Fatal(updates)
	}
}

func TestProgressFinishWaitsForInflight(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	finished := make(chan error, 1)
	var states []string
	p := newProgress(context.Background(), time.Millisecond, func(_ context.Context, s cardState) error {
		state, text := s.State, s.Result
		if state == "运行中" && len(s.Summaries) > 0 {
			text = s.Summaries[len(s.Summaries)-1]
		}
		_ = text
		states = append(states, state)
		if state == "运行中" {
			close(entered)
			<-release
		}
		return nil
	})
	p.report("摘要")
	p.start()
	<-entered
	p.report("待丢弃")
	go func() { finished <- p.finish("已完成", "结论") }()
	select {
	case <-finished:
		t.Fatal("did not wait for inflight update")
	default:
	}
	close(release)
	if err := <-finished; err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(states, []string{"运行中", "已完成"}) {
		t.Fatal(states)
	}
}

func TestCardUsesPlainText(t *testing.T) {
	text := "<at id=all>测试</at> [文字](https://example.invalid)"
	content, err := cardContent(cardState{State: "运行中", Summaries: []string{text}})
	if err != nil {
		t.Fatal(err)
	}
	var card struct {
		Elements []struct {
			Text struct {
				Tag     string `json:"tag"`
				Content string `json:"content"`
			} `json:"text"`
		} `json:"elements"`
	}
	if err = json.Unmarshal([]byte(content), &card); err != nil {
		t.Fatal(err)
	}
	if len(card.Elements) != 1 || card.Elements[0].Text.Tag != "plain_text" || card.Elements[0].Text.Content != text {
		t.Fatal("invalid card text")
	}
	if _, err = cardContent(cardState{State: "已完成", Result: strings.Repeat("长", 30000)}); err == nil {
		t.Fatal("accepted oversized card")
	}
}
