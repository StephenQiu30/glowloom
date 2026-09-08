ARG GO_IMAGE=golang:1.26-alpine
FROM ${GO_IMAGE} AS builder

WORKDIR /src
COPY go.mod go.sum ./
COPY api ./api
COPY cmd ./cmd
COPY internal ./internal
ARG BUILD_VERSION=development
ARG BUILD_COMMIT=unknown
ARG BUILD_TIME=unknown
RUN CGO_ENABLED=0 go build -trimpath \
    -ldflags="-s -w \
    -X github.com/StephenQiu30/lanverse/backend/internal/bootstrap.BuildVersion=${BUILD_VERSION} \
    -X github.com/StephenQiu30/lanverse/backend/internal/bootstrap.BuildCommit=${BUILD_COMMIT} \
    -X github.com/StephenQiu30/lanverse/backend/internal/bootstrap.BuildTime=${BUILD_TIME}" \
    -o /out/lanverse ./cmd

FROM alpine:3.23

RUN apk add --no-cache ca-certificates su-exec \
    && addgroup -S lanverse \
    && adduser -S -G lanverse lanverse

COPY --from=builder /out/lanverse /usr/local/bin/lanverse
RUN <<'BUILD_ENTRYPOINT'
cat > /usr/local/bin/docker-entrypoint.sh <<'ENTRYPOINT_SCRIPT'
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

exec su-exec lanverse:lanverse "$@"
ENTRYPOINT_SCRIPT
chmod 0755 /usr/local/bin/docker-entrypoint.sh
BUILD_ENTRYPOINT

EXPOSE 8686

HEALTHCHECK --interval=10s --timeout=3s --start-period=10s --retries=12 \
    CMD su-exec lanverse:lanverse wget --quiet --spider http://127.0.0.1:8686/healthz || exit 1

ENTRYPOINT ["/usr/local/bin/docker-entrypoint.sh"]
CMD ["/usr/local/bin/lanverse"]
