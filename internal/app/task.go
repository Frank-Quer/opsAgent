package app

import (
	"context"
	"errors"
	"log"
	"time"

	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"

	"opsagent/internal/codex"
)

// taskInput is captured while holding bot.mu; only the worker mutates its request.
type taskInput struct {
	message      *larkim.EventMessage
	request      codex.Request
	sessionKey   string
	environment  string
	modelCommand bool
}

func (b *bot) executeTask(ctx context.Context, cancel context.CancelFunc, task taskInput) {
	id := value(task.message.MessageId)
	defer b.wg.Done()
	defer cancel()
	defer func() {
		b.mu.Lock()
		b.busy = false
		b.activeUser = ""
		b.activeCancel = nil
		b.activeCard = ""
		b.activeChat = ""
		b.activeStopped = false
		b.activeRevoked = false
		b.mu.Unlock()
	}()
	if task.modelCommand {
		output := b.modelCommand(b.ctx, value(task.message.ChatId), task.request.Prompt, task.request.Directory)
		b.sendTaskResult(id, output)
		return
	}
	receiptCtx, receiptCancel := context.WithTimeout(ctx, 15*time.Second)
	cardID, err := b.createCard(receiptCtx, id)
	receiptCancel()
	if err != nil {
		log.Print("排查卡片发送失败，未启动任务")
		return
	}
	b.mu.Lock()
	b.activeCard, b.activeChat = cardID, value(task.message.ChatId)
	b.mu.Unlock()
	p := newProgress(b.ctx, 3*time.Second, func(ctx context.Context, state cardState) error {
		b.mu.Lock()
		revoked := b.activeRevoked
		stopped := b.activeStopped
		b.mu.Unlock()
		if stopped {
			state.State, state.Result = "已停止", "任务已终止。"
		}
		if revoked {
			state = cardState{State: "已停止", Result: "授权已撤销，任务已停止。"}
		}
		return b.updateCard(ctx, cardID, state)
	})
	p.start()
	var images []string
	if b.prepare != nil {
		var cleanup func()
		task.request.Prompt, images, cleanup = b.prepare(ctx, task.message, task.request.Prompt)
		defer cleanup()
	}
	var next, output string
	if ctx.Err() != nil {
		err = ctx.Err()
	} else {
		var result codex.Result
		task.request.Prompt = environmentPrompt(task.environment) + task.request.Prompt
		task.request.Images = images
		result, err = b.run(ctx, task.request, p.report)
		next, output = result.Session, result.Output
	}
	state := "已完成"
	if err != nil {
		state, output = "失败", "任务失败，请稍后重试。未自动重跑。"
		if errors.Is(err, context.DeadlineExceeded) {
			state, output = "超时", "任务超过 10 分钟，已停止；不会自动重跑。"
		} else if errors.Is(err, context.Canceled) || b.ctx.Err() != nil {
			state, output = "已停止", "任务已停止。"
		}
	}
	b.mu.Lock()
	revoked := b.activeRevoked
	stopped := b.activeStopped
	b.activeCard = ""
	if !revoked && !stopped && err == nil {
		b.sessions[task.sessionKey] = next
	}
	b.mu.Unlock()
	if stopped {
		state, output = "已停止", "任务已终止。"
	}
	if revoked {
		state, output = "已停止", "授权已撤销，任务已停止。"
		next = ""
	}
	if p.finish(state, output, next) != nil && b.ctx.Err() == nil {
		b.sendTaskResult(id, output)
	}
}

func (b *bot) sendTaskResult(id, output string) {
	for _, part := range splitReply(output, 3000) {
		b.mu.Lock()
		revoked := b.activeRevoked
		b.mu.Unlock()
		if revoked || !b.send(id, part) {
			return
		}
	}
}
