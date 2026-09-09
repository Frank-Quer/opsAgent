package app

import (
	"errors"
	"strings"
)

type config struct{ appID, secret, allowed string }

func loadConfig(getenv func(string) string) (config, error) {
	appID := strings.TrimSpace(getenv("FEISHU_APP_ID"))
	secret := strings.TrimSpace(getenv("FEISHU_APP_SECRET"))
	allowed := strings.TrimSpace(getenv("FEISHU_ALLOWED_OPEN_ID"))
	if !validID(appID, "cli_") || secret == "" {
		return config{}, errors.New("请设置 FEISHU_APP_ID 和 FEISHU_APP_SECRET")
	}
	if allowed != "" && !validID(allowed, "ou_") {
		return config{}, errors.New("FEISHU_ALLOWED_OPEN_ID 格式无效")
	}
	return config{appID: appID, secret: secret, allowed: allowed}, nil
}
