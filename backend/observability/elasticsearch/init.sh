#!/usr/bin/env bash
set -euo pipefail

elasticsearch_url=${ELASTICSEARCH_URL:-http://elasticsearch:9200}
elasticsearch_username=${ELASTICSEARCH_INIT_USERNAME:-${ELASTICSEARCH_USERNAME:-}}
elasticsearch_password=${ELASTICSEARCH_INIT_PASSWORD:-${ELASTICSEARCH_PASSWORD:-}}
config_root=${LANVERSE_OBSERVABILITY_CONFIG_ROOT:-/usr/share/lanverse-observability}
ilm_policy_path=${LANVERSE_ILM_POLICY_PATH:-${config_root}/ilm-policy.json}
index_template_path=${LANVERSE_LOG_INDEX_TEMPLATE_PATH:-${config_root}/lanverse-logs-template.json}
application_alias=lanverse-logs-application
application_backing=lanverse-logs-application-write
dead_letter_alias=lanverse-logs-dead-letter
dead_letter_backing=lanverse-logs-dead-letter-write
blocked_legacy_index=
curl_options=(--connect-timeout 5 --max-time 60 --fail --silent --show-error)
curl_probe_options=(--connect-timeout 3 --max-time 5 --fail --silent --show-error)

if [[ -n "${elasticsearch_username}" || -n "${elasticsearch_password}" ]]; then
  if [[ -z "${elasticsearch_username}" || -z "${elasticsearch_password}" ]]; then
    echo "ELASTICSEARCH_INIT_USERNAME and ELASTICSEARCH_INIT_PASSWORD must be configured together" >&2
    exit 1
  fi
fi

elasticsearch_curl() {
  if [[ -n "${elasticsearch_username}" ]]; then
    curl --user "${elasticsearch_username}:${elasticsearch_password}" "$@"
    return
  fi
  curl "$@"
}

restore_legacy_writes() {
  if [[ -n "${blocked_legacy_index}" ]]; then
    elasticsearch_curl "${curl_options[@]}" \
      --request PUT "${elasticsearch_url}/${blocked_legacy_index}/_settings" \
      --header 'Content-Type: application/json' \
      --data-binary '{"index.blocks.write":false}' >/dev/null || true
  fi
}
trap restore_legacy_writes EXIT

alias_exists() {
  elasticsearch_curl "${curl_probe_options[@]}" \
    "${elasticsearch_url}/_alias/$1" >/dev/null 2>&1
}

index_exists() {
  elasticsearch_curl "${curl_probe_options[@]}" \
    --head "${elasticsearch_url}/$1" >/dev/null 2>&1
}

create_backing() {
  local backing=$1
  local body=$2
  if index_exists "${backing}"; then
    return
  fi
  elasticsearch_curl "${curl_options[@]}" \
    --request PUT "${elasticsearch_url}/${backing}" \
    --header 'Content-Type: application/json' \
    --data-binary "${body}" >/dev/null
}

reindex_without_overwrite() {
  local source=$1
  local destination=$2
  local response
  response=$(elasticsearch_curl "${curl_options[@]}" \
    --request POST "${elasticsearch_url}/_reindex?refresh=true&wait_for_completion=true" \
    --header 'Content-Type: application/json' \
    --data-binary "{\"conflicts\":\"proceed\",\"source\":{\"index\":\"${source}\"},\"dest\":{\"index\":\"${destination}\",\"op_type\":\"create\"}}")
  if ! grep -Eq '"failures"[[:space:]]*:[[:space:]]*\[[[:space:]]*\]' <<<"${response}"; then
    echo "failed to reindex formal Elasticsearch index ${source}" >&2
    exit 1
  fi
}

document_count() {
  local index=$1
  local response
  response=$(elasticsearch_curl "${curl_options[@]}" \
    "${elasticsearch_url}/${index}/_count")
  sed -n 's/.*"count":\([0-9][0-9]*\).*/\1/p' <<<"${response}"
}

add_formal_alias() {
  local alias=$1
  local backing=$2
  elasticsearch_curl "${curl_options[@]}" \
    --request POST "${elasticsearch_url}/_aliases" \
    --header 'Content-Type: application/json' \
    --data-binary "{\"actions\":[{\"add\":{\"index\":\"${backing}\",\"alias\":\"${alias}\",\"is_write_index\":true}}]}" >/dev/null
}

migrate_formal_index() {
  local alias=$1
  local backing=$2
  local backing_body=$3
  local source_count
  local destination_count

  if alias_exists "${alias}"; then
    return
  fi
  create_backing "${backing}" "${backing_body}"
  if ! index_exists "${alias}"; then
    add_formal_alias "${alias}" "${backing}"
    return
  fi

  reindex_without_overwrite "${alias}" "${backing}"
  elasticsearch_curl "${curl_options[@]}" \
    --request PUT "${elasticsearch_url}/${alias}/_settings" \
    --header 'Content-Type: application/json' \
    --data-binary '{"index.blocks.write":true}' >/dev/null
  blocked_legacy_index=${alias}
  reindex_without_overwrite "${alias}" "${backing}"
  source_count=$(document_count "${alias}")
  destination_count=$(document_count "${backing}")
  if [[ -z "${source_count}" || "${source_count}" != "${destination_count}" ]]; then
    echo "formal Elasticsearch index migration count mismatch for ${alias}" >&2
    exit 1
  fi

  elasticsearch_curl "${curl_options[@]}" \
    --request POST "${elasticsearch_url}/_aliases" \
    --header 'Content-Type: application/json' \
    --data-binary "{\"actions\":[{\"remove_index\":{\"index\":\"${alias}\"}},{\"add\":{\"index\":\"${backing}\",\"alias\":\"${alias}\",\"is_write_index\":true}}]}" >/dev/null
  blocked_legacy_index=
}

elasticsearch_curl "${curl_options[@]}" \
  --request PUT "${elasticsearch_url}/_ilm/policy/lanverse-logs-application-30d" \
  --header 'Content-Type: application/json' \
  --data-binary "@${ilm_policy_path}" >/dev/null

elasticsearch_curl "${curl_options[@]}" \
  --request PUT "${elasticsearch_url}/_index_template/lanverse-logs-application" \
  --header 'Content-Type: application/json' \
  --data-binary "@${index_template_path}" >/dev/null

migrate_formal_index "${application_alias}" "${application_backing}" '{}'
migrate_formal_index "${dead_letter_alias}" "${dead_letter_backing}" \
  '{"settings":{"index.lifecycle.name":"lanverse-logs-application-30d","index.lifecycle.rollover_alias":"lanverse-logs-dead-letter","number_of_shards":1,"number_of_replicas":0},"mappings":{"dynamic":"strict","properties":{"@timestamp":{"type":"date_nanos"},"schema_version":{"type":"keyword"},"error_code":{"type":"keyword"},"raw_sha256":{"type":"keyword"},"tags":{"type":"keyword"}}}}'

elasticsearch_curl "${curl_options[@]}" \
  --request PUT "${elasticsearch_url}/${dead_letter_alias}/_settings" \
  --header 'Content-Type: application/json' \
  --data-binary '{"index.number_of_replicas":0,"index.lifecycle.rollover_alias":"lanverse-logs-dead-letter"}' >/dev/null
