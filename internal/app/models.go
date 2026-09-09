package app

import (
	"context"
	"errors"
	"strings"

	"opsagent/internal/codex"
)

func (s *workspaceStore) model(app, chat string) string { return s.bindings[app][chat].Model }
func (s *workspaceStore) setModel(app, chat, model, effort string) error {
	b := s.bindings[app][chat]
	if b.Directory == "" {
		return errors.New("请先设置工作区。")
	}
	if !codex.ValidModel(model) {
		return errors.New("模型 ID 格式无效。")
	}
	if !codex.ValidReasoningEffort(effort) {
		return errors.New("思考强度无效。")
	}
	b.Model = model
	b.ReasoningEffort = effort
	return s.save(app, chat, b)
}

func (b *bot) modelCommand(ctx context.Context, chat, text, dir string) string {
	b.mu.Lock()
	current := b.workspaces.model(b.appID, chat)
	effort := b.workspaces.bindings[b.appID][chat].ReasoningEffort
	b.mu.Unlock()
	currentModel, currentEffort := current, effort
	if currentModel == "" {
		currentModel = "跟随本机 Codex 配置"
	}
	if currentEffort == "" {
		currentEffort = "跟随本机 Codex 配置"
	}
	currentSettings := "当前模型：" + currentModel + "\n思考强度：" + currentEffort
	if text == "/model" {
		return currentSettings
	}
	if text == "/model clear" {
		b.mu.Lock()
		defer b.mu.Unlock()
		if e := b.workspaces.setModel(b.appID, chat, "", ""); e != nil {
			return e.Error()
		}
		return "已恢复本机 Codex 默认模型和思考强度，保留当前会话。"
	}
	if text != "/models" && !strings.HasPrefix(text, "/model set ") {
		return "用法：/models、/model、/model set 模型ID [思考强度]、/model clear。"
	}
	if dir == "" {
		return "请先设置工作区。"
	}
	model := ""
	effort = ""
	if text != "/models" {
		fields := strings.Fields(strings.TrimPrefix(text, "/model set "))
		if len(fields) < 1 || len(fields) > 2 {
			return "用法：/model set 模型ID [思考强度]。"
		}
		model = fields[0]
		if len(fields) == 2 {
			effort = fields[1]
		}
		if !codex.ValidReasoningEffort(effort) {
			return "思考强度无效，请使用 /models 查看支持的值。"
		}
	}
	if text != "/models" && (model == "" || !codex.ValidModel(model)) {
		return "模型 ID 格式无效。"
	}
	models, e := b.models(ctx, dir)
	if e != nil {
		return e.Error()
	}
	if text == "/models" {
		if len(models) == 0 {
			return currentSettings + "\n\nCodex 未返回模型列表。"
		}
		lines := []string{currentSettings, "", "Codex 模型列表："}
		for _, m := range models {
			line := m.Model
			if m.Default {
				line += "（默认）"
			}
			var efforts []string
			for _, option := range m.SupportedReasoningEfforts {
				if option.ReasoningEffort != "" && codex.ValidReasoningEffort(option.ReasoningEffort) {
					efforts = append(efforts, option.ReasoningEffort)
				}
			}
			if len(efforts) > 0 {
				line += "；思考强度：" + strings.Join(efforts, ", ")
			}
			lines = append(lines, line)
		}
		return strings.Join(lines, "\n")
	}
	found := false
	for _, m := range models {
		if m.Model == model {
			found = true
			if effort != "" {
				supported := false
				for _, option := range m.SupportedReasoningEfforts {
					if option.ReasoningEffort == effort {
						supported = true
					}
				}
				if !supported {
					return "该模型不支持此思考强度，请使用 /models 查看支持的值。"
				}
			}
		}
	}
	if !found {
		return "模型未出现在 Codex 列表中，请使用 /models 查看。"
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if e = b.workspaces.setModel(b.appID, chat, model, effort); e != nil {
		return e.Error()
	}
	if effort == "" {
		effort = "跟随本机 Codex 配置"
	}
	return "已设置模型：" + model + "，思考强度：" + effort + "，保留当前会话。"
}
