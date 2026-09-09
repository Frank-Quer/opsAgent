package app

import (
	"context"

	"github.com/larksuite/oapi-sdk-go/v3/event/dispatcher/callback"
)

func (b *bot) handleCardAction(_ context.Context, e *callback.CardActionTriggerEvent) (*callback.CardActionTriggerResponse, error) {
	response := &callback.CardActionTriggerResponse{Toast: &callback.Toast{Type: "error", Content: "无效的终止请求。"}}
	if e == nil || e.EventV2Base == nil || e.EventV2Base.Header == nil || e.EventV2Base.Header.AppID != b.appID || e.Event == nil {
		return response, nil
	}
	event := e.Event
	if event.Operator == nil || event.Context == nil || event.Action == nil || event.Action.Tag != "button" || event.Action.Value["action"] != "stop_task" || !validID(event.Operator.OpenID, "ou_") || !validID(event.Context.OpenMessageID, "om_") || !validID(event.Context.OpenChatID, "oc_") {
		return response, nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	user := event.Operator.OpenID
	if user != b.allowed && (user != b.activeUser || !b.users.has(b.appID, user)) {
		response.Toast.Content = "仅任务发起人或管理员可终止任务。"
		return response, nil
	}
	if b.closed || !b.busy || b.activeCancel == nil || b.activeCard != event.Context.OpenMessageID || b.activeChat != event.Context.OpenChatID {
		response.Toast.Content = "该任务已结束或已失效。"
		return response, nil
	}
	b.activeStopped = true
	b.activeCancel()
	response.Toast = &callback.Toast{Type: "success", Content: "已请求终止任务。"}
	return response, nil
}
