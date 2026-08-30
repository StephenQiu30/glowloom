#!/usr/bin/env bash
set -euo pipefail

kibana_url=${KIBANA_URL:-http://kibana:5601}
kibana_username=${KIBANA_USERNAME:-}
kibana_password=${KIBANA_PASSWORD:-}
data_view_id=lanverse-logs-application-v1
curl_auth=()
if [[ -n "${kibana_username}" || -n "${kibana_password}" ]]; then
  if [[ -z "${kibana_username}" || -z "${kibana_password}" ]]; then
    echo "KIBANA_USERNAME and KIBANA_PASSWORD must be configured together" >&2
    exit 1
  fi
  curl_auth=(--user "${kibana_username}:${kibana_password}")
fi

if curl "${curl_auth[@]}" --fail --silent "${kibana_url}/api/data_views/data_view/${data_view_id}" >/dev/null; then
  exit 0
fi

curl "${curl_auth[@]}" --fail --silent --show-error \
  --request POST "${kibana_url}/api/data_views/data_view" \
  --header 'Content-Type: application/json' \
  --header 'kbn-xsrf: lanverse-observability-init' \
  --data-binary '{"data_view":{"id":"lanverse-logs-application-v1","name":"Lanverse Application Logs","title":"lanverse-logs-application-v1-*","timeFieldName":"@timestamp","allowNoIndex":true}}' >/dev/null
