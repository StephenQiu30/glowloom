#!/bin/sh
set -eu

minio_port="${LANVERSE_E2E_MINIO_PORT:-19010}"
data_dir="$(mktemp -d "${TMPDIR:-/tmp}/lanverse-e2e-minio.XXXXXX")"
state_file="${TMPDIR:-/tmp}/lanverse-e2e-minio-${minio_port}.path"
child_pid=""

printf '%s\n' "$data_dir" > "$state_file"

cleanup() {
  if [ -n "$child_pid" ]; then
    kill "$child_pid" >/dev/null 2>&1 || true
    wait "$child_pid" >/dev/null 2>&1 || true
  fi
  find "$data_dir" -depth -delete
  rm -f "$state_file"
}
trap cleanup EXIT INT TERM

env \
  MINIO_ROOT_USER=lanverse-e2e \
  MINIO_ROOT_PASSWORD=lanverse-e2e-only \
  minio server --quiet --address "127.0.0.1:$minio_port" --console-address "127.0.0.1:19011" "$data_dir" &
child_pid="$!"
wait "$child_pid"
