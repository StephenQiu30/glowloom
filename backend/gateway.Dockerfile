# syntax=docker/dockerfile:1
FROM golang:1.26.11-alpine AS build
ARG GOPROXY="https://goproxy.cn,direct"
ENV GOPROXY=${GOPROXY}
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/gateway ./cmd/gateway

FROM debian:12-slim AS runtime
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates curl \
    && rm -rf /var/lib/apt/lists/*
COPY --from=build /out/gateway /usr/local/bin/gateway
USER 65532:65532
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/gateway"]
