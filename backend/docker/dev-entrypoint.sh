#!/bin/sh
set -eu

# Keep host .env loopback addresses, Kafka advertised listeners and S3 signing
# origins intact. These listeners exist only inside each development container.
for port in ${LV_HOST_SERVICE_PORTS:-5432 6379 7233 9000 9092}; do
  case "$port" in
    ''|*[!0-9]*) echo "Invalid host service port" >&2; exit 1 ;;
  esac
  socat "TCP-LISTEN:$port,bind=127.0.0.1,reuseaddr,fork" "TCP:host.docker.internal:$port" &
done

# The single-workspace API deliberately requires a loopback listener. Compose
# exposes this bridge listener only on the host's loopback interface.
if [ "${LV_DEV_API_PROXY:-false}" = "true" ]; then
  socat TCP-LISTEN:18080,bind=0.0.0.0,reuseaddr,fork TCP:127.0.0.1:8080 &
fi

exec air -c /src/docker/air.toml -- "$@"
