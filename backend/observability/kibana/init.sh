#!/usr/bin/env bash
set -euo pipefail

kibana_url=${KIBANA_URL:-http://kibana:5601}
data_view_id=lanverse-logs-application-v1

if curl --fail --silent "${kibana_url}/api/data_views/data_view/${data_view_id}" >/dev/null; then
  exit 0
fi

curl --fail --silent --show-error \
  --request POST "${kibana_url}/api/data_views/data_view" \
  --header 'Content-Type: application/json' \
  --header 'kbn-xsrf: lanverse-observability-init' \
  --data-binary '{"data_view":{"id":"lanverse-logs-application-v1","name":"Lanverse Application Logs","title":"lanverse-logs-application-v1-*","timeFieldName":"@timestamp","allowNoIndex":true}}' >/dev/null
