#!/bin/sh
set -eu

repository_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
agent_root="$repository_root/agent"
backend_root="$repository_root/backend"
frontend_root="$repository_root/frontend"
python_runtime="$agent_root/.venv/bin/python"
agent_api_port=${AGENT_API_PORT:-8787}
backend_api_port=${API_PORT:-8686}
frontend_port=${FRONTEND_PORT:-8123}
runtime_directory=$(mktemp -d "${TMPDIR:-/tmp}/lanverse-local-development.XXXXXX")
backend_binary="$runtime_directory/lanverse-api"
process_ids=""

cleanup() {
    trap - EXIT INT TERM HUP
    for process_id in $process_ids; do
        kill "$process_id" 2>/dev/null || true
    done
    for process_id in $process_ids; do
        wait "$process_id" 2>/dev/null || true
    done
    rm -rf -- "$runtime_directory"
}

trap cleanup EXIT INT TERM HUP

require_command() {
    command -v "$1" >/dev/null 2>&1 || {
        printf '缺少本机命令：%s\n' "$1" >&2
        exit 1
    }
}

require_available_port() {
    port_number=$1
    service_name=$2
    if command -v lsof >/dev/null 2>&1 \
        && lsof -nP -iTCP:"$port_number" -sTCP:LISTEN >/dev/null 2>&1; then
        printf '%s 端口 %s 已被占用。\n' "$service_name" "$port_number" >&2
        exit 1
    fi
}

wait_for_endpoint() {
    endpoint=$1
    service_name=$2
    attempt=1
    while [ "$attempt" -le 60 ]; do
        if curl --fail --silent --show-error "$endpoint" >/dev/null 2>&1; then
            return 0
        fi
        sleep 1
        attempt=$((attempt + 1))
    done
    printf '%s 未在 60 秒内就绪：%s\n' "$service_name" "$endpoint" >&2
    return 1
}

require_command curl
require_command go
require_command node
test -x "$python_runtime" || {
    printf '缺少 Agent Python 环境：%s\n' "$python_runtime" >&2
    exit 1
}
test -f "$repository_root/.env" || {
    printf '缺少本机配置：请先从 .env.example 创建仓库根目录 .env。\n' >&2
    exit 1
}
test -d "$frontend_root/node_modules" || {
    printf '缺少 Frontend 依赖：请先在 frontend/ 执行 npm ci。\n' >&2
    exit 1
}

require_available_port "$agent_api_port" "Agent 兼容 API"
require_available_port "$backend_api_port" "Go Backend"
require_available_port "$frontend_port" "Frontend"

printf '构建 Go Backend……\n'
(cd "$backend_root" && go build -trimpath -o "$backend_binary" ./cmd/api)

printf '启动 Python Agent 兼容 API 并准备隔离开发数据库：http://127.0.0.1:%s\n' "$agent_api_port"
(
    cd "$agent_root"
    API_HOST=127.0.0.1 API_PORT="$agent_api_port" \
        exec "$python_runtime" -m app.runtime.local_development
) &
agent_process_id=$!
process_ids="$process_ids $agent_process_id"
wait_for_endpoint "http://127.0.0.1:$agent_api_port/readyz" "Python Agent 兼容运行时"

printf '启动 Go Backend：http://127.0.0.1:%s\n' "$backend_api_port"
API_HOST=127.0.0.1 \
API_PORT="$backend_api_port" \
LEGACY_API_URL="http://127.0.0.1:$agent_api_port" \
    "$backend_binary" &
backend_process_id=$!
process_ids="$process_ids $backend_process_id"
wait_for_endpoint "http://127.0.0.1:$backend_api_port/readyz" "Go Backend"

printf '启动 Frontend：http://127.0.0.1:%s\n' "$frontend_port"
(
    cd "$frontend_root"
    PORT="$frontend_port" \
        exec node ./node_modules/next/dist/bin/next dev \
        --hostname 127.0.0.1 \
        --port "$frontend_port"
) &
frontend_process_id=$!
process_ids="$process_ids $frontend_process_id"
wait_for_endpoint "http://127.0.0.1:$frontend_port/" "Frontend"

printf '本机开发服务已就绪；按 Ctrl+C 统一停止。\n'
while :; do
    for process_id in $process_ids; do
        if ! kill -0 "$process_id" 2>/dev/null; then
            if wait "$process_id"; then
                exit_code=0
            else
                exit_code=$?
            fi
            printf '开发进程 %s 已退出，状态码 %s。\n' "$process_id" "$exit_code" >&2
            exit "$exit_code"
        fi
    done
    sleep 1
done
