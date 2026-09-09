package app

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"opsagent/internal/codex"
)

type workspaceStore struct {
	path     string
	bindings map[string]map[string]workspaceBinding
}

func loadWorkspaces(path string) (*workspaceStore, error) {
	s := &workspaceStore{path: path, bindings: map[string]map[string]workspaceBinding{}}
	data, e := os.ReadFile(path)
	if os.IsNotExist(e) {
		return s, nil
	}
	if e != nil {
		return nil, errors.New("工作区配置读取失败")
	}
	if json.Unmarshal(data, &s.bindings) != nil || s.bindings == nil {
		return nil, errors.New("工作区配置损坏")
	}
	for app, chats := range s.bindings {
		if !validID(app, "cli_") || chats == nil {
			return nil, errors.New("工作区配置无效")
		}
		for chat, binding := range chats {
			dir := binding.Directory
			if !validID(chat, "oc_") || !filepath.IsAbs(dir) || strings.ContainsAny(dir, "\x00\r\n") || !validEnvironment(binding.Environment) || !codex.ValidModel(binding.Model) {
				return nil, errors.New("工作区配置无效")
			}
		}
	}
	return s, nil
}
func (s *workspaceStore) get(app, chat string) string { return s.bindings[app][chat].Directory }

// Caller holds the bot mutex; commit memory only after the atomic file replacement.
func (s *workspaceStore) set(app, chat, dir string) error {
	binding := s.bindings[app][chat]
	if binding.Directory != dir {
		binding = workspaceBinding{Directory: dir}
	}
	return s.save(app, chat, binding)
}
func (s *workspaceStore) save(app, chat string, binding workspaceBinding) error {
	next := make(map[string]map[string]workspaceBinding, len(s.bindings))
	for a, chats := range s.bindings {
		next[a] = map[string]workspaceBinding{}
		for c, p := range chats {
			next[a][c] = p
		}
	}
	if next[app] == nil {
		next[app] = map[string]workspaceBinding{}
	}
	if binding.Directory == "" {
		delete(next[app], chat)
	} else {
		next[app][chat] = binding
	}
	data, e := json.MarshalIndent(next, "", "  ")
	if e != nil {
		return errors.New("工作区配置保存失败")
	}
	parent := filepath.Dir(s.path)
	if e = os.MkdirAll(parent, 0700); e != nil {
		return errors.New("工作区配置保存失败")
	}
	if e = os.Chmod(parent, 0700); e != nil {
		return errors.New("工作区配置保存失败")
	}
	if writeAtomic(s.path, data) != nil {
		return errors.New("工作区配置保存失败")
	}
	s.bindings = next
	return nil
}
func normalizeWorkspace(path string) (string, error) {
	if !filepath.IsAbs(path) || strings.ContainsAny(path, "\x00\r\n") {
		return "", errors.New("请提供本机已有目录的绝对路径")
	}
	resolved, e := filepath.EvalSymlinks(filepath.Clean(path))
	if e != nil {
		return "", errors.New("工作区目录不存在或不可访问，请重新设置")
	}
	f, e := os.Open(resolved)
	if e != nil {
		return "", errors.New("工作区目录不可访问，请重新设置")
	}
	defer f.Close()
	info, e := f.Stat()
	if e != nil || !info.IsDir() {
		return "", errors.New("工作区必须是目录")
	}
	if _, e = f.Readdirnames(1); e != nil && !errors.Is(e, io.EOF) {
		return "", errors.New("工作区目录不可读取")
	}
	return resolved, nil
}
func (b *bot) projectCommand(chat, text string) string {
	dir := b.workspaces.get(b.appID, chat)
	switch {
	case text == "/project":
		if dir == "" {
			return "尚未设置项目，请使用 /project set /绝对路径。"
		}
		return "当前项目：" + filepath.Base(dir)
	case text == "/project clear":
		if dir == "" {
			return "尚未设置项目。"
		}
		if e := b.workspaces.set(b.appID, chat, ""); e != nil {
			return e.Error()
		}
		b.clearChatSessions(chat)
		return "已解除项目绑定，并清空当前会话。"
	case strings.HasPrefix(text, "/project set "):
		next, e := normalizeWorkspace(strings.TrimSpace(strings.TrimPrefix(text, "/project set ")))
		if e != nil {
			return e.Error()
		}
		if next == dir {
			return "当前项目：" + filepath.Base(dir)
		}
		if e = b.workspaces.set(b.appID, chat, next); e != nil {
			return e.Error()
		}
		b.clearChatSessions(chat)
		return "已切换到项目：" + filepath.Base(next) + "，已清空当前会话。"
	default:
		return "用法：/project、/project set /绝对路径、/project clear。"
	}
}
func (b *bot) clearChatSessions(chat string) {
	suffix := ":" + chat + "\x00"
	for key := range b.sessions {
		if strings.HasSuffix(strings.SplitN(key, "\x00", 2)[0]+"\x00", suffix) {
			delete(b.sessions, key)
		}
	}
}
