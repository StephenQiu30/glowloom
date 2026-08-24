ARG GO_IMAGE=golang:1.26-alpine
FROM ${GO_IMAGE} AS builder

WORKDIR /src
COPY go.mod ./go.mod
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/lanverse-api ./cmd/api

FROM alpine:3.23

RUN apk add --no-cache ca-certificates \
    && addgroup -S lanverse \
    && adduser -S -G lanverse lanverse

COPY --from=builder /out/lanverse-api /usr/local/bin/lanverse-api

USER lanverse
EXPOSE 8686

HEALTHCHECK --interval=10s --timeout=3s --start-period=10s --retries=12 \
    CMD wget --quiet --spider http://127.0.0.1:8686/healthz || exit 1

ENTRYPOINT ["/usr/local/bin/lanverse-api"]
