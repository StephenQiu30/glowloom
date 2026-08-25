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
    -ldflags="-s -w -X main.buildVersion=${BUILD_VERSION} -X main.buildCommit=${BUILD_COMMIT} -X main.buildTime=${BUILD_TIME}" \
    -o /out/lanverse-api ./cmd/api
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" \
    -o /out/lanverse-workflow-worker ./cmd/workflow-worker

FROM alpine:3.23

RUN apk add --no-cache ca-certificates \
    && addgroup -S lanverse \
    && adduser -S -G lanverse lanverse

COPY --from=builder /out/lanverse-api /usr/local/bin/lanverse-api
COPY --from=builder /out/lanverse-workflow-worker /usr/local/bin/lanverse-workflow-worker

USER lanverse
EXPOSE 8686

HEALTHCHECK --interval=10s --timeout=3s --start-period=10s --retries=12 \
    CMD wget --quiet --spider http://127.0.0.1:8686/healthz || exit 1

ENTRYPOINT ["/usr/local/bin/lanverse-api"]
