# syntax=docker/dockerfile:1
FROM golang:1.26.9-bookworm AS dev
ENV CGO_ENABLED=0 GOMAXPROCS=2
RUN apt-get update && apt-get install -y --no-install-recommends ffmpeg curl socat \
    && rm -rf /var/lib/apt/lists/*
RUN go install github.com/air-verse/air@v1.67.4
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -p 2 -o /tmp/lanverse-air/lanverse ./cmd/lanverse
ENTRYPOINT ["sh", "/src/docker/dev-entrypoint.sh"]
CMD ["--role=api"]

FROM golang:1.26.9-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/lanverse ./cmd/lanverse

FROM debian:12-slim
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates ffmpeg \
    && rm -rf /var/lib/apt/lists/* \
    && mkdir -p /work \
    && chown 65532:65532 /work
COPY --from=build /out/lanverse /usr/local/bin/lanverse
COPY internal/media/adapter/gltf/schema/COPYING.adoc internal/media/adapter/gltf/schema/LICENSE.adoc internal/media/adapter/gltf/schema/PROVENANCE.md /usr/share/lanverse/licenses/gltf/
COPY internal/media/adapter/gltf/schema/LICENSES/ /usr/share/lanverse/licenses/gltf/LICENSES/
COPY internal/mediatool/adapter/ffmpeg/fonts/OFL.txt internal/mediatool/adapter/ffmpeg/fonts/PROVENANCE.md /usr/share/lanverse/licenses/noto-cjk/
ENV TMPDIR=/work
USER 65532:65532
EXPOSE 8080 8081 8082
ENTRYPOINT ["/usr/local/bin/lanverse"]
CMD ["--role=api"]
