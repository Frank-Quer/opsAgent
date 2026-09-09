package codex

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

// Runner invokes the locally installed Codex CLI.
type Runner struct{ Binary string }

// Request describes one turn, including an optional existing session.
type Request struct {
	Directory       string
	Session         string
	Prompt          string
	ReasoningEffort string
	Model           string
	Images          []string
}

// Result contains only the session identifier and final answer.
type Result struct{ Session, Output string }

func (r *Runner) Run(ctx context.Context, req Request, progress func(string)) (Result, error) {
	if !ValidReasoningEffort(req.ReasoningEffort) {
		return Result{}, errors.New("思考强度无效")
	}
	args := []string{"-a", "never", "exec", "--sandbox", "danger-full-access"}
	if req.Session != "" {
		args = append(args, "resume")
	}
	args = append(args, "--json", "--skip-git-repo-check")
	if req.Model != "" {
		args = append(args, "--model", req.Model)
	}
	if req.ReasoningEffort != "" {
		args = append(args, "-c", fmt.Sprintf("model_reasoning_effort=%q", req.ReasoningEffort))
	}
	for _, path := range req.Images {
		args = append(args, "--image", path)
	}
	if req.Session != "" {
		args = append(args, req.Session)
	}
	args = append(args, "-")
	cmd := command(ctx, r.Binary, req.Directory, args...)
	cmd.Stdin = strings.NewReader(opsInstruction + "\n\n用户请求：\n" + req.Prompt)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return Result{}, errors.New("无法读取 Codex 输出")
	}
	if err = cmd.Start(); err != nil {
		return Result{}, errors.New("无法启动 Codex")
	}
	next, output, parseErr := parseCodex(stdout, req.Session, progress)
	if parseErr != nil {
		_ = cmd.Cancel()
	}
	waitErr := cmd.Wait()
	if ctx.Err() != nil {
		return Result{}, ctx.Err()
	}
	if parseErr != nil {
		return Result{}, parseErr
	}
	if waitErr != nil {
		return Result{}, fmt.Errorf("Codex 退出异常（exit=%d），请检查本机登录和配置", cmd.ProcessState.ExitCode())
	}
	return Result{Session: next, Output: output}, nil
}

func parseCodex(reader io.Reader, session string, progress func(string)) (string, string, error) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64*1024), 2*1024*1024)
	var output string
	completed, failed := false, false
	for scanner.Scan() {
		var event struct {
			Type     string `json:"type"`
			ThreadID string `json:"thread_id"`
			Item     struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"item"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			return "", "", errors.New("Codex 事件格式无效")
		}
		switch event.Type {
		case "thread.started":
			if event.ThreadID == "" || len(event.ThreadID) > 128 || strings.HasPrefix(event.ThreadID, "-") || strings.ContainsAny(event.ThreadID, "\r\n\x00") {
				return "", "", errors.New("Codex 会话 ID 无效")
			}
			session = event.ThreadID
		case "item.completed":
			if event.Item.Type == "agent_message" && event.Item.Text != "" {
				if len(event.Item.Text) > 1024*1024 {
					return "", "", errors.New("Codex 回答超过 1 MB")
				}
				// exec 会发送中间进度消息；仅保留最后一条回答。
				output = event.Item.Text
				if progress != nil {
					progress(output)
				}
			}
		case "turn.completed":
			completed = true
		case "turn.failed":
			failed = true
		}
	}
	if scanner.Err() != nil {
		return "", "", errors.New("读取 Codex 事件失败")
	}
	if failed || !completed || session == "" || strings.TrimSpace(output) == "" {
		return "", "", errors.New("Codex 未成功完成回答，请检查本机登录、模型和工具配置")
	}
	return session, output, nil
}
