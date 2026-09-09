package app

import (
	"errors"
	"os"
	"regexp"
	"strings"
)

// Only update a simple assignment; never execute the configuration to inspect it.
func saveBinding(path, id string) error {
	if !validID(id, "ou_") {
		return errors.New("绑定用户 ID 无效")
	}
	data, e := os.ReadFile(path)
	if e != nil && !os.IsNotExist(e) {
		return errors.New("绑定配置读取失败")
	}
	lines := strings.Split(string(data), "\n")
	assignment := regexp.MustCompile(`^\s*(?:export\s+)?FEISHU_ALLOWED_OPEN_ID\s*=(.*)$`)
	index := -1
	for i, line := range lines {
		m := assignment.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		if index >= 0 {
			return errors.New("绑定配置有重复项，请检查 .env")
		}
		index = i
		v := strings.TrimSpace(m[1])
		if v != "" && v != "''" && v != `""` && v != id && v != "'"+id+"'" && v != `"`+id+`"` {
			return errors.New("已有绑定配置，不能覆盖")
		}
	}
	line := "export FEISHU_ALLOWED_OPEN_ID='" + id + "'"
	if index >= 0 {
		lines[index] = line
	} else {
		lines = append(lines, line)
	}
	data = []byte(strings.TrimRight(strings.Join(lines, "\n"), "\n") + "\n")
	if writeAtomic(path, data) != nil {
		return errors.New("绑定配置保存失败")
	}
	return nil
}
