#!/bin/sh
set -eu

broker_port="${LANVERSE_E2E_KAFKA_PORT:-19092}"
controller_port="${LANVERSE_E2E_KAFKA_CONTROLLER_PORT:-19093}"
ready_port="${LANVERSE_E2E_KAFKA_READY_PORT:-19094}"
runtime_dir="$(mktemp -d)"
config_file="$runtime_dir/server.properties"
kafka_pid=""

cleanup() {
  if [ -n "$kafka_pid" ]; then
    kill "$kafka_pid" 2>/dev/null || true
    wait "$kafka_pid" 2>/dev/null || true
  fi
  rm -rf "$runtime_dir"
}
trap cleanup EXIT INT TERM

sed \
  -e "s|^controller.quorum.bootstrap.servers=.*|controller.quorum.bootstrap.servers=127.0.0.1:$controller_port|" \
  -e "s|^listeners=.*|listeners=PLAINTEXT://127.0.0.1:$broker_port,CONTROLLER://127.0.0.1:$controller_port|" \
  -e "s|^advertised.listeners=.*|advertised.listeners=PLAINTEXT://127.0.0.1:$broker_port,CONTROLLER://127.0.0.1:$controller_port|" \
  -e "s|^log.dirs=.*|log.dirs=$runtime_dir/logs|" \
  /opt/homebrew/etc/kafka/server.properties > "$config_file"

cluster_id="$(kafka-storage random-uuid)"
kafka-storage format --standalone --cluster-id "$cluster_id" --config "$config_file" >/dev/null
kafka-server-start "$config_file" >"$runtime_dir/kafka.log" 2>&1 &
kafka_pid="$!"

attempt=0
until kafka-topics --bootstrap-server "127.0.0.1:$broker_port" --list >/dev/null 2>&1; do
  attempt=$((attempt + 1))
  if [ "$attempt" -ge 60 ]; then
    sed -n '1,240p' "$runtime_dir/kafka.log"
    exit 1
  fi
  sleep 1
done

kafka-topics --bootstrap-server "127.0.0.1:$broker_port" --create --if-not-exists --topic lanverse.io.v1 >/dev/null
kafka-topics --bootstrap-server "127.0.0.1:$broker_port" --create --if-not-exists --topic lanverse.media.v1 >/dev/null
python3 -m http.server "$ready_port" --bind 127.0.0.1 >/dev/null 2>&1
