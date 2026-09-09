package app

import (
	"context"
	"errors"
	"strings"

	"opsagent/internal/codex"
)

func (s *workspaceStore) model(app, chat string) string { return s.bindings[app][chat].Model }
func (s *workspaceStore) setModel(app, chat, model string) error {
	b := s.bindings[app][chat]
	if b.Directory == "" {
		return errors.New("请先设置工作区。")
	}
	if !codex.ValidModel(model) {
		return errors.New("模型 ID 格式无效。")
	}
	b.Model = model
	return s.save(app, chat, b)
}

func (b *bot) modelCommand(ctx context.Context, chat, text, dir string) string {
	b.mu.Lock()
	current := b.workspaces.model(b.appID, chat)
	b.mu.Unlock()
	if text == "/model" {
		if current == "" {
			return "模型：跟随本机 Codex 配置。"
		}
		return "当前模型：" + current
	}
	if text == "/model clear" {
		b.mu.Lock()
		defer b.mu.Unlock()
		if e := b.workspaces.setModel(b.appID, chat, ""); e != nil {
			return e.Error()
		}
		return "已恢复本机 Codex 默认模型，保留当前会话。"
	}
	if text != "/models" && !strings.HasPrefix(text, "/model set ") {
		return "用法：/models、/model、/model set 模型ID、/model clear。"
	}
	if dir == "" {
		return "请先设置工作区。"
	}
	model := strings.TrimSpace(strings.TrimPrefix(text, "/model set "))
	if text != "/models" && (model == "" || !codex.ValidModel(model)) {
		return "模型 ID 格式无效。"
	}
	models, e := b.models(ctx, dir)
	if e != nil {
		return e.Error()
	}
	if text == "/models" {
		if len(models) == 0 {
			return "Codex 未返回模型列表。"
		}
		lines := []string{"Codex 模型列表："}
		for _, m := range models {
			line := m.Model
			if m.Default {
				line += "（默认）"
			}
			lines = append(lines, line)
		}
		return strings.Join(lines, "\n")
	}
	found := false
	for _, m := range models {
		if m.Model == model {
			found = true
		}
	}
	if !found {
		return "模型未出现在 Codex 列表中，请使用 /models 查看。"
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if e = b.workspaces.setModel(b.appID, chat, model); e != nil {
		return e.Error()
	}
	return "已设置模型：" + model + "，保留当前会话。"
}
