#!/usr/bin/env bash
set -euo pipefail

: "${KAFKA_ADMIN_PASSWORD:?KAFKA_ADMIN_PASSWORD is required}"

bootstrap_server=${KAFKA_BOOTSTRAP_SERVERS:-kafka:19092}
properties=/tmp/lanverse-kafka-admin.properties
printf '%s\n' \
  'security.protocol=SASL_PLAINTEXT' \
  'sasl.mechanism=PLAIN' \
  "sasl.jaas.config=org.apache.kafka.common.security.plain.PlainLoginModule required username=\"admin\" password=\"${KAFKA_ADMIN_PASSWORD}\";" \
  >"${properties}"

create_topic() {
  local topic=$1
  local retention=$2
  /opt/kafka/bin/kafka-topics.sh \
    --bootstrap-server "${bootstrap_server}" \
    --command-config "${properties}" \
    --create --if-not-exists --topic "${topic}" \
    --partitions 1 --replication-factor 1 \
    --config cleanup.policy=delete --config "retention.ms=${retention}"
}

grant_topic() {
  local principal=$1
  local topic=$2
  shift 2
  /opt/kafka/bin/kafka-acls.sh \
    --bootstrap-server "${bootstrap_server}" \
    --command-config "${properties}" \
    --add --allow-principal "User:${principal}" \
    --topic "${topic}" "$@"
}

grant_group() {
  local principal=$1
  local group=$2
  /opt/kafka/bin/kafka-acls.sh \
    --bootstrap-server "${bootstrap_server}" \
    --command-config "${properties}" \
    --add --allow-principal "User:${principal}" \
    --group "${group}" --operation Read --operation Describe
}

grant_idempotent_write() {
  local principal=$1
  /opt/kafka/bin/kafka-acls.sh \
    --bootstrap-server "${bootstrap_server}" \
    --command-config "${properties}" \
    --add --allow-principal "User:${principal}" \
    --cluster --operation IdempotentWrite
}

script_topic=lanverse.business.script-version.v1
script_dlq=lanverse.business.script-version.dlq.v1
storygraph_topic=lanverse.business.storygraph-version.v1
storygraph_dlq=lanverse.business.storygraph-version.dlq.v1
create_topic "${script_topic}" 604800000
create_topic "${script_dlq}" 2592000000
create_topic "${storygraph_topic}" 604800000
create_topic "${storygraph_dlq}" 2592000000

for topic in "${script_topic}" "${script_dlq}" "${storygraph_topic}" "${storygraph_dlq}"; do
  grant_topic event_worker "${topic}" --operation Read --operation Write --operation Describe
done
grant_group event_worker lanverse.search-projector.v1
grant_idempotent_write event_worker
