package app

import (
	"context"
	"sync"
	"time"
)

type cardState struct {
	State     string
	Summaries []string
	Result    string
	Elapsed   time.Duration
	Frame     int
}

// 一个协程串行刷新卡片，回调只维护最近四条，给最终回答去重预留一条。
type progress struct {
	ctx       context.Context
	interval  time.Duration
	update    func(context.Context, cardState) error
	mu        sync.Mutex
	summaries []string
	started   time.Time
	stop      chan struct{}
	done      chan struct{}
	err       error
}

func newProgress(ctx context.Context, interval time.Duration, update func(context.Context, cardState) error) *progress {
	return &progress{ctx: ctx, interval: interval, update: update, started: time.Now(), stop: make(chan struct{}), done: make(chan struct{})}
}

func (p *progress) report(text string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if text == "" || (len(p.summaries) > 0 && p.summaries[len(p.summaries)-1] == text) {
		return
	}
	p.summaries = append(p.summaries, text)
	if len(p.summaries) > 4 {
		p.summaries = append([]string(nil), p.summaries[len(p.summaries)-4:]...)
	}
}

func (p *progress) snapshot(state, result string, frame int) cardState {
	p.mu.Lock()
	defer p.mu.Unlock()
	items := p.summaries
	if state == "已完成" && len(items) > 0 && items[len(items)-1] == result {
		items = items[:len(items)-1]
	}
	if len(items) > 3 {
		items = items[len(items)-3:]
	}
	return cardState{State: state, Result: result, Frame: frame, Elapsed: time.Since(p.started), Summaries: append([]string(nil), items...)}
}
func (p *progress) send(s cardState) error {
	ctx, cancel := context.WithTimeout(p.ctx, 15*time.Second)
	defer cancel()
	return p.update(ctx, s)
}

func (p *progress) start() {
	go func() {
		defer close(p.done)
		timer := time.NewTicker(p.interval)
		defer timer.Stop()
		frame := 0
		for {
			select {
			case <-p.stop:
				return
			case <-p.ctx.Done():
				p.err = p.ctx.Err()
				return
			case <-timer.C:
				select {
				case <-p.stop:
					return
				default:
				}
				frame = (frame + 1) % 3
				if p.err = p.send(p.snapshot("运行中", "", frame)); p.err != nil {
					return
				}
				timer.Reset(p.interval)
			}
		}
	}()
}

func (p *progress) finish(state, text string) error {
	close(p.stop)
	<-p.done
	if p.err != nil {
		return p.err
	}
	return p.send(p.snapshot(state, text, 0))
}
