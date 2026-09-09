package codex

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

type Model struct {
	Model                     string            `json:"model"`
	Default                   bool              `json:"isDefault"`
	SupportedReasoningEfforts []ReasoningEffort `json:"supportedReasoningEfforts"`
}

type ReasoningEffort struct {
	ReasoningEffort string `json:"reasoningEffort"`
}

func ValidReasoningEffort(v string) bool {
	switch v {
	case "", "none", "minimal", "low", "medium", "high", "xhigh":
		return true
	}
	return false
}

func ValidModel(v string) bool {
	if len(v) > 128 {
		return false
	}
	for _, r := range v {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("-_.:/", r)) {
			return false
		}
	}
	return v == "" || v[0] != '-'
}

// A short-lived stdio app-server only discovers models; it never starts an agent turn.
func ListModels(ctx context.Context, binary, dir string) ([]Model, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	cmd := command(ctx, binary, dir, "app-server", "--listen", "stdio://")
	in, e := cmd.StdinPipe()
	if e != nil {
		return nil, errors.New("模型列表进程启动失败")
	}
	out, e := cmd.StdoutPipe()
	if e != nil {
		return nil, errors.New("模型列表进程启动失败")
	}
	if e = cmd.Start(); e != nil {
		return nil, errors.New("模型列表进程启动失败")
	}
	defer func() { in.Close(); cmd.Cancel(); cmd.Wait() }()
	enc := json.NewEncoder(in)
	scanner := bufio.NewScanner(out)
	scanner.Buffer(make([]byte, 4096), 1024*1024)
	call := func(id int, method string, params any) (json.RawMessage, error) {
		if enc.Encode(map[string]any{"id": id, "method": method, "params": params}) != nil {
			return nil, errors.New("模型列表请求失败")
		}
		for scanner.Scan() {
			var r struct {
				ID     *int            `json:"id"`
				Result json.RawMessage `json:"result"`
				Error  json.RawMessage `json:"error"`
			}
			if json.Unmarshal(scanner.Bytes(), &r) != nil {
				return nil, errors.New("模型列表响应无效")
			}
			if r.ID != nil && *r.ID == id {
				if len(r.Error) > 0 && string(r.Error) != "null" {
					return nil, errors.New("Codex 无法获取模型列表")
				}
				return r.Result, nil
			}
		}
		return nil, errors.New("模型列表读取失败或超时")
	}
	if _, e = call(1, "initialize", map[string]any{"clientInfo": map[string]string{"name": "opsagent", "version": "1"}}); e != nil {
		return nil, e
	}
	if enc.Encode(map[string]string{"method": "initialized"}) != nil {
		return nil, errors.New("模型列表初始化失败")
	}
	var models []Model
	cursor := ""
	seen := map[string]bool{}
	for id := 2; ; id++ {
		params := map[string]any{"limit": 100, "includeHidden": false}
		if cursor != "" {
			params["cursor"] = cursor
		}
		raw, e := call(id, "model/list", params)
		if e != nil {
			return nil, e
		}
		var page struct {
			Data []Model `json:"data"`
			Next string  `json:"nextCursor"`
		}
		if json.Unmarshal(raw, &page) != nil {
			return nil, errors.New("模型列表格式无效")
		}
		for _, m := range page.Data {
			if m.Model == "" || !ValidModel(m.Model) {
				return nil, errors.New("模型列表包含无效 ID")
			}
			models = append(models, m)
		}
		if page.Next == "" {
			break
		}
		if seen[page.Next] {
			return nil, errors.New("模型列表分页异常")
		}
		seen[page.Next] = true
		cursor = page.Next
	}
	return models, nil
}
