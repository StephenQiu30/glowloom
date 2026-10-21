# syntax=docker/dockerfile:1
FROM golang:1.26.10-bookworm AS dev
ENV CGO_ENABLED=0 GOMAXPROCS=2 GOCACHE=/go/build-cache
RUN apt-get update && apt-get install -y --no-install-recommends ffmpeg curl socat \
    && rm -rf /var/lib/apt/lists/*
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# Build the mounted source on every start; never fall back to an image binary.
# The entrypoint lives outside /src, which Compose mounts read-only.
COPY --chmod=755 <<'ENTRYPOINT' /usr/local/bin/glowloom-dev-entrypoint
#!/bin/sh
set -eu
# Optional private settings for this existing local container; explicit env wins.
if [ -z "${LV_ENV_FILE:-}" ] && [ -r /run/secrets/identity-runtime.env ]; then
  export LV_ENV_FILE=/run/secrets/identity-runtime.env
fi
mkdir -p /tmp/glowloom-dev
printf '%s\n' '[glowloom-dev] Building current source in /src...' >&2
if ! go build -mod=readonly -p 2 -o /tmp/glowloom-dev/glowloom ./cmd/glowloom; then
  printf '%s\n' '[glowloom-dev] Build failed; application was not started. Fix the source and restart.' >&2
  exit 1
fi
for port in ${LV_HOST_SERVICE_PORTS:-5432 6379 7233 9000 9092}; do
  case "$port" in
    ''|*[!0-9]*) echo "Invalid host service port" >&2; exit 1 ;;
  esac
  socat "TCP-LISTEN:$port,bind=127.0.0.1,reuseaddr,fork" "TCP:host.docker.internal:$port" &
done
if [ "${LV_DEV_API_PROXY:-false}" = "true" ]; then
  socat TCP-LISTEN:18080,bind=0.0.0.0,reuseaddr,fork TCP:127.0.0.1:8080 &
fi
printf '%s\n' '[glowloom-dev] Build succeeded; starting current source.' >&2
exec /tmp/glowloom-dev/glowloom "$@"
ENTRYPOINT
ENTRYPOINT ["/usr/local/bin/glowloom-dev-entrypoint"]
CMD ["--role=api"]

FROM golang:1.26.10-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/glowloom ./cmd/glowloom

FROM debian:12-slim
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates ffmpeg \
    && rm -rf /var/lib/apt/lists/* \
    && mkdir -p /work \
    && chown 65532:65532 /work
COPY --from=build /out/glowloom /usr/local/bin/glowloom
COPY internal/media/adapter/gltf/schema/COPYING.adoc internal/media/adapter/gltf/schema/LICENSE.adoc internal/media/adapter/gltf/schema/PROVENANCE.md /usr/share/glowloom/licenses/gltf/
COPY internal/media/adapter/gltf/schema/LICENSES/ /usr/share/glowloom/licenses/gltf/LICENSES/
COPY internal/mediatool/adapter/ffmpeg/fonts/OFL.txt internal/mediatool/adapter/ffmpeg/fonts/PROVENANCE.md /usr/share/glowloom/licenses/noto-cjk/
ENV TMPDIR=/work
USER 65532:65532
EXPOSE 8080 8081 8082
ENTRYPOINT ["/usr/local/bin/glowloom"]
CMD ["--role=api"]
