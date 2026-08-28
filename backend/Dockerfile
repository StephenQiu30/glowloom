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

RUN apk add --no-cache ca-certificates \
    && addgroup -S lanverse \
    && adduser -S -G lanverse lanverse

COPY --from=builder /out/lanverse /usr/local/bin/lanverse

USER lanverse
EXPOSE 8686

HEALTHCHECK --interval=10s --timeout=3s --start-period=10s --retries=12 \
    CMD wget --quiet --spider http://127.0.0.1:8686/healthz || exit 1

ENTRYPOINT ["/usr/local/bin/lanverse"]
