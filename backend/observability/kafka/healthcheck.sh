#!/usr/bin/env bash
set -euo pipefail

: "${KAFKA_ADMIN_PASSWORD:?KAFKA_ADMIN_PASSWORD is required}"

properties=/tmp/lanverse-kafka-health.properties
printf '%s\n' \
  'security.protocol=SASL_PLAINTEXT' \
  'sasl.mechanism=PLAIN' \
  "sasl.jaas.config=org.apache.kafka.common.security.plain.PlainLoginModule required username=\"admin\" password=\"${KAFKA_ADMIN_PASSWORD}\";" \
  >"${properties}"

exec /opt/kafka/bin/kafka-broker-api-versions.sh \
  --bootstrap-server 127.0.0.1:9092 \
  --command-config "${properties}"
