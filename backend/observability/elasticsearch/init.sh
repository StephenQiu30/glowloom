#!/usr/bin/env bash
set -euo pipefail

elasticsearch_url=${ELASTICSEARCH_URL:-http://elasticsearch:9200}
config_root=/usr/share/lanverse-observability

curl --fail --silent --show-error \
  --request PUT "${elasticsearch_url}/_ilm/policy/lanverse-logs-application-30d" \
  --header 'Content-Type: application/json' \
  --data-binary "@${config_root}/ilm-policy.json" >/dev/null

curl --fail --silent --show-error \
  --request PUT "${elasticsearch_url}/_index_template/lanverse-logs-application-v1" \
  --header 'Content-Type: application/json' \
  --data-binary "@${config_root}/lanverse-logs-template.json" >/dev/null

if ! curl --fail --silent "${elasticsearch_url}/_alias/lanverse-logs-application-v1" >/dev/null; then
  curl --fail --silent --show-error \
    --request PUT "${elasticsearch_url}/lanverse-logs-application-v1-000001" \
    --header 'Content-Type: application/json' \
    --data-binary '{"aliases":{"lanverse-logs-application-v1":{"is_write_index":true}}}' >/dev/null
fi
