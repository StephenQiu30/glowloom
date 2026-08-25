#!/bin/sh
set -eu

postgres_port="${LANVERSE_E2E_POSTGRES_PORT:-15432}"
data_dir="$(mktemp -d "${TMPDIR:-/tmp}/lanverse-e2e-postgres.XXXXXX")"
state_file="${TMPDIR:-/tmp}/lanverse-e2e-postgres-${postgres_port}.path"
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

initdb -D "$data_dir" --auth=trust --encoding=UTF8 --no-locale --username=lanverse_e2e >/dev/null
postgres -D "$data_dir" -h 127.0.0.1 -p "$postgres_port" &
child_pid="$!"
wait "$child_pid"
