#!/bin/bash
set -euo pipefail

cd "$(dirname "$0")"
project_dir="$(pwd -P)"

case "${1:-}" in
  start|stop) ;;
  *) echo "用法：$0 {start|stop}"; exit 1 ;;
esac

# 按工作目录识别实例，也能关闭之前手动启动的机器人。
service_pids=()
while IFS= read -r pid; do
  cwd="$(lsof -a -p "$pid" -d cwd -Fn 2>/dev/null | sed -n 's/^n//p' || true)"
  if [[ "$cwd" == "$project_dir" ]]; then
    service_pids+=("$pid")
  fi
done < <(pgrep -x opsagent || true)

if [[ "$1" == stop ]]; then
  if [[ ${#service_pids[@]} -eq 0 ]]; then
    echo "服务未运行。"
    exit 0
  fi
  for pid in "${service_pids[@]}"; do
    kill -TERM "$pid"
  done
  for ((attempt=0; attempt<150; attempt++)); do
    running=false
    for pid in "${service_pids[@]}"; do
      if kill -0 "$pid" 2>/dev/null; then running=true; fi
    done
    if [[ "$running" == false ]]; then
      echo "服务已关闭，防休眠将随进程退出自动解除。"
      exit 0
    fi
    sleep 0.1
  done
  echo "服务尚未退出，未强制终止，请检查进程。" >&2
  exit 1
fi

if [[ ${#service_pids[@]} -gt 0 ]]; then
  echo "服务已运行。若需应用新版本，请先 stop 再 start。"
  exit 0
fi

# .env 为本人维护的 shell 配置，与 README 的 source .env 用法一致。
if [[ -f .env ]]; then source .env; fi
if [[ -z "${FEISHU_APP_ID:-}" || -z "${FEISHU_APP_SECRET:-}" ]]; then
  echo "请在 .env 中填写飞书配置，或先导出 FEISHU_APP_ID 和 FEISHU_APP_SECRET。" >&2
  exit 1
fi
export FEISHU_APP_ID FEISHU_APP_SECRET
export FEISHU_ALLOWED_OPEN_ID="${FEISHU_ALLOWED_OPEN_ID:-}"
if [[ ! -x bin/opsagent ]]; then
  echo "请先执行：go build -o bin/opsagent ." >&2
  exit 1
fi
command -v codex >/dev/null || { echo "PATH 中找不到 codex。" >&2; exit 1; }
command -v python3 >/dev/null || { echo "PATH 中找不到 python3。" >&2; exit 1; }
[[ -x /usr/bin/caffeinate ]] || { echo "此脚本需要 macOS caffeinate。" >&2; exit 1; }

umask 077
mkdir -p .run
chmod 700 .run
log_file=".run/service.log"
touch "$log_file"
chmod 600 "$log_file"
# 每次启动重置日志，避免将旧的连接成功提示误认为本次启动成功。
: > "$log_file"
service_pid="$(python3 - "$log_file" <<'PYTHON'
import os
import subprocess
import sys

with open(sys.argv[1], "ab", buffering=0) as log:
    service = subprocess.Popen(
        ["./bin/opsagent"], stdin=subprocess.DEVNULL, stdout=log, stderr=log,
        start_new_session=True,
    )
try:
    subprocess.Popen(
        ["/usr/bin/caffeinate", "-i", "-w", str(service.pid)],
        stdin=subprocess.DEVNULL, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL,
        start_new_session=True,
    )
except OSError:
    service.terminate()
    service.wait(timeout=15)
    sys.exit("防休眠进程启动失败，已停止服务。")
print(service.pid)
PYTHON
)"

for ((attempt=0; attempt<200; attempt++)); do
  if ! kill -0 "$service_pid" 2>/dev/null; then
    echo "服务启动失败，请查看 $project_dir/$log_file。" >&2
    exit 1
  fi
  if /usr/bin/grep -q '飞书长连接已就绪' "$log_file"; then
    echo "服务已启动，飞书连接正常，已启用防闲置休眠。"
    exit 0
  fi
  sleep 0.1
done
echo "进程已启动，但尚未确认飞书连接，请查看 $project_dir/$log_file。" >&2
exit 1
