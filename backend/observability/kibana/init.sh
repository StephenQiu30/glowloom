#!/usr/bin/env bash
set -euo pipefail

kibana_url=${KIBANA_URL:-http://kibana:5601}
kibana_username=${KIBANA_USERNAME:-}
kibana_password=${KIBANA_PASSWORD:-}
data_view_id=lanverse-logs-application
curl_options=(--connect-timeout 5 --max-time 30 --fail --silent --show-error)
if [[ -n "${kibana_username}" || -n "${kibana_password}" ]]; then
  if [[ -z "${kibana_username}" || -z "${kibana_password}" ]]; then
    echo "KIBANA_USERNAME and KIBANA_PASSWORD must be configured together" >&2
    exit 1
  fi
fi

kibana_curl() {
  if [[ -n "${kibana_username}" ]]; then
    curl --user "${kibana_username}:${kibana_password}" "$@"
    return
  fi
  curl "$@"
}

if kibana_curl "${curl_options[@]}" "${kibana_url}/api/data_views/data_view/${data_view_id}" >/dev/null 2>&1; then
  exit 0
fi

kibana_curl "${curl_options[@]}" \
  --request POST "${kibana_url}/api/data_views/data_view" \
  --header 'Content-Type: application/json' \
  --header 'kbn-xsrf: lanverse-observability-init' \
  --data-binary '{"data_view":{"id":"lanverse-logs-application","name":"Lanverse Application Logs","title":"lanverse-logs-application-*","timeFieldName":"@timestamp","allowNoIndex":true}}' >/dev/null
