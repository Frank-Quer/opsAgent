package app

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"

	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
)

type userStore struct {
	path    string
	members map[string]map[string]bool
}

func loadUsers(path string) (*userStore, error) {
	s := &userStore{path: path, members: map[string]map[string]bool{}}
	data, e := os.ReadFile(path)
	if os.IsNotExist(e) {
		return s, nil
	}
	if e != nil {
		return nil, errors.New("用户配置读取失败")
	}
	if json.Unmarshal(data, &s.members) != nil || s.members == nil {
		return nil, errors.New("用户配置损坏")
	}
	for app, users := range s.members {
		if !validID(app, "cli_") || users == nil {
			return nil, errors.New("用户配置无效")
		}
		for id, enabled := range users {
			if !validID(id, "ou_") || !enabled {
				return nil, errors.New("用户配置无效")
			}
		}
	}
	return s, nil
}
func (s *userStore) has(app, user string) bool { return s != nil && s.members[app][user] }
func (s *userStore) set(app, user string, add bool) error {
	next := map[string]map[string]bool{}
	for a, users := range s.members {
		next[a] = map[string]bool{}
		for u, v := range users {
			next[a][u] = v
		}
	}
	if next[app] == nil {
		next[app] = map[string]bool{}
	}
	if add {
		next[app][user] = true
	} else {
		delete(next[app], user)
	}
	parent := filepath.Dir(s.path)
	if os.MkdirAll(parent, 0700) != nil || os.Chmod(parent, 0700) != nil {
		return errors.New("用户配置保存失败")
	}
	data, e := json.MarshalIndent(next, "", "  ")
	if e != nil {
		return errors.New("用户配置保存失败")
	}
	if writeAtomic(s.path, data) != nil {
		return errors.New("用户配置保存失败")
	}
	s.members = next
	return nil
}
func (b *bot) userCommand(text string, mentions []*larkim.MentionEvent) string {
	if text == "/users" {
		ids := []string{}
		for id := range b.users.members[b.appID] {
			if id != b.allowed {
				ids = append(ids, id)
			}
		}
		sort.Strings(ids)
		return "管理员：" + b.allowed + "\n授权用户：\n" + strings.Join(ids, "\n")
	}
	fields := strings.Fields(text)
	if len(fields) != 3 || (fields[1] != "add" && fields[1] != "remove") {
		return "用法：/users、/users add @成员或用户ID、/users remove @成员或用户ID。"
	}
	target := fields[2]
	for _, m := range mentions {
		if m != nil && value(m.Key) == target {
			if m.Id == nil || value(m.MentionedType) == "bot" {
				return "请选择一名用户。"
			}
			target = value(m.Id.OpenId)
		}
	}
	if !validID(target, "ou_") || target == b.botID {
		return "请指定一名用户的 @ 提及或 ou_ 用户 ID。"
	}
	if target == b.allowed {
		return "管理员始终有权限，不能添加或删除自身。"
	}
	add := fields[1] == "add"
	if b.users.has(b.appID, target) == add {
		if add {
			return "该用户已获授权。"
		}
		return "该用户未获授权。"
	}
	if e := b.users.set(b.appID, target, add); e != nil {
		return e.Error()
	}
	if add {
		return "已添加用户：" + target
	}
	for key := range b.sessions {
		if strings.HasPrefix(key, target+":") {
			delete(b.sessions, key)
		}
	}
	if b.activeUser == target && b.activeCancel != nil {
		b.activeRevoked = true
		b.activeCancel()
	}
	return "已移除用户：" + target
}

// Caller holds the bot mutex.
func (b *bot) replyCommand(id, text string) {
	b.wg.Add(1)
	go func() {
		defer b.wg.Done()
		for _, part := range splitReply(text, 3000) {
			if !b.send(id, part) {
				break
			}
		}
	}()
}
