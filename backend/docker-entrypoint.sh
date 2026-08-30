#!/bin/sh
set -eu
umask 077

mounted_root_key=/run/secrets/lanverse_media_provider_master_key_source
runtime_root_key=/run/secrets/lanverse_media_provider_master_key

if [ -f "$mounted_root_key" ] && [ -s "$mounted_root_key" ]; then
  if [ "$(stat -f -c %T /run/secrets)" != tmpfs ]; then
    echo "media Provider root key requires a tmpfs runtime directory" >&2
    exit 1
  fi
  install -o lanverse -g lanverse -m 0400 "$mounted_root_key" "$runtime_root_key"
else
  if [ -e "$runtime_root_key" ]; then
    unlink "$runtime_root_key"
  fi
fi

if ! /usr/share/lanverse-observability/elasticsearch-init.sh; then
  echo "Elasticsearch formal index initialization failed; Backend will start and retry its search runtime independently" >&2
elif ! /usr/share/lanverse-observability/kibana-init.sh; then
  echo "Kibana data view initialization failed; Backend will start without blocking API availability" >&2
fi

unset ELASTICSEARCH_INIT_USERNAME ELASTICSEARCH_INIT_PASSWORD KIBANA_USERNAME KIBANA_PASSWORD

exec su-exec lanverse:lanverse "$@"
