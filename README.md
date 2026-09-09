# opsAgent

通过飞书私聊或群内 @ 调用本机 Codex，结合项目代码、shell、SSH 和已有工具排查问题，支持连续追问和进度卡片。

## 功能

- 群聊与私聊独立绑定工作区，按用户隔离会话。
- 管理员绑定与成员授权，支持撤销授权并取消当前任务。
- 按聊天设置默认环境和模型，重启后保留配置。
- 读取引用消息、群历史与图片，在同一张卡片展示摘要和结论。

## 环境要求

- macOS 或 Linux；Go 版本以 [go.mod](go.mod) 为准，当前要求 1.26.3 或更高。
- 已安装并登录的 Codex CLI；原有集成验证版本为 `0.153.4`。依赖 `exec --json` 和 `app-server` 模型列表协议。
- 飞书企业自建应用，已启用机器人、长连接事件及相应权限。
- `make check` 需要 Make 和 Python 3；`service.sh` 另需 macOS 的 `caffeinate`、`pgrep` 和 `lsof`。

CI 配置覆盖 macOS/Linux；Windows 不在支持范围内。程序使用 Unix socket 和 Unix 进程组。

## 快速启动

在项目根目录执行：

```sh
go build -o bin/opsagent .
cp .env.example .env
chmod 600 .env
# 编辑 .env，填写应用凭据；首次绑定时将 FEISHU_ALLOWED_OPEN_ID 留空。
source .env
./bin/opsagent
```

程序不会自动加载 `.env`。飞书后台配置长连接，订阅 `im.message.receive_v1` 并发布应用后，由本人私聊发送 `/bind`，再发送 `/project set /项目绝对路径`。完成后可提问，使用 `/new` 开始新会话。

完整权限清单、所有命令、配置位置和故障限制见 [使用指南](docs/usage.md)。macOS 可用 `./service.sh start` 和 `./service.sh stop` 后台运行；Linux 使用上述前台启动方式。

## 执行与授权边界

机器人复用本机 Codex 登录与工具配置，以 `--sandbox danger-full-access` 和 `-a never` 执行任务，可操作本机与 SSH 可访问的环境。请仅向可信成员授权，并按工作区规则明确任务权限。

- 首次 `/bind` 采用先到先得；也可在启动前设置管理员 `FEISHU_ALLOWED_OPEN_ID`。
- 同时仅执行一个任务，10 分钟超时；忙碌消息不排队，失败不自动重跑。
- 飞书凭据不传入 Codex 子进程；群历史和引用内容作为分析数据。
- 回复保密依赖提示词约束，不是程序强制过滤；本机 Codex 会话可保留原始上下文。

## 开发

```sh
make check  # 格式、静态检查、脚本语法、竞态测试与构建
make test   # 仅竞态测试，60 秒硬超时
make build  # 构建 bin/opsagent
```

测试使用模拟接口和子进程，无需飞书凭据或 Codex 登录。模块结构、兼容性要求和贡献流程见 [贡献指南](CONTRIBUTING.md)。

## 许可证

[MIT](LICENSE) © 2026 opsAgent contributors。
