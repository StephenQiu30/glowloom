#!/usr/bin/env bash
set -euo pipefail

elasticsearch_url=${ELASTICSEARCH_URL:-http://elasticsearch:9200}
elasticsearch_username=${ELASTICSEARCH_USERNAME:-}
elasticsearch_password=${ELASTICSEARCH_PASSWORD:-}
config_root=${LANVERSE_OBSERVABILITY_CONFIG_ROOT:-/usr/share/lanverse-observability}
ilm_policy_path=${LANVERSE_ILM_POLICY_PATH:-${config_root}/ilm-policy.json}
index_template_path=${LANVERSE_LOG_INDEX_TEMPLATE_PATH:-${config_root}/lanverse-logs-template.json}
curl_auth=()
if [[ -n "${elasticsearch_username}" || -n "${elasticsearch_password}" ]]; then
  if [[ -z "${elasticsearch_username}" || -z "${elasticsearch_password}" ]]; then
    echo "ELASTICSEARCH_USERNAME and ELASTICSEARCH_PASSWORD must be configured together" >&2
    exit 1
  fi
  curl_auth=(--user "${elasticsearch_username}:${elasticsearch_password}")
fi

curl "${curl_auth[@]}" --fail --silent --show-error \
  --request PUT "${elasticsearch_url}/_ilm/policy/lanverse-logs-application-30d" \
  --header 'Content-Type: application/json' \
  --data-binary "@${ilm_policy_path}" >/dev/null

curl "${curl_auth[@]}" --fail --silent --show-error \
  --request PUT "${elasticsearch_url}/_index_template/lanverse-logs-application-v1" \
  --header 'Content-Type: application/json' \
  --data-binary "@${index_template_path}" >/dev/null

if ! curl "${curl_auth[@]}" --fail --silent "${elasticsearch_url}/_alias/lanverse-logs-application-v1" >/dev/null; then
  curl "${curl_auth[@]}" --fail --silent --show-error \
    --request PUT "${elasticsearch_url}/lanverse-logs-application-v1-000001" \
    --header 'Content-Type: application/json' \
    --data-binary '{"aliases":{"lanverse-logs-application-v1":{"is_write_index":true}}}' >/dev/null
fi

if ! curl "${curl_auth[@]}" --fail --silent "${elasticsearch_url}/lanverse-logs-dead-letter-v1" >/dev/null; then
  curl "${curl_auth[@]}" --fail --silent --show-error \
    --request PUT "${elasticsearch_url}/lanverse-logs-dead-letter-v1" \
    --header 'Content-Type: application/json' \
    --data-binary '{"settings":{"index.lifecycle.name":"lanverse-logs-application-30d"},"mappings":{"dynamic":"strict","properties":{"@timestamp":{"type":"date_nanos"},"schema_version":{"type":"keyword"},"error_code":{"type":"keyword"},"raw_sha256":{"type":"keyword"},"tags":{"type":"keyword"}}}}' >/dev/null
fi
