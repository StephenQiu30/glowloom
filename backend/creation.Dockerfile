FROM golang:1.26.11-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/creation ./cmd/creation

FROM alpine:3.23
RUN apk add --no-cache ca-certificates \
    && mkdir /work && chown 65532:65532 /work && chmod 700 /work
ENV TMPDIR=/work
USER 65532:65532
EXPOSE 9092 8083
HEALTHCHECK --interval=10s --timeout=3s --start-period=5s --retries=3 \
    CMD wget -q -O /dev/null "http://${GLOWLOOM_CREATION_HTTP_ADDR:-127.0.0.1:8083}/health/ready" || exit 1
COPY --from=build /out/creation /usr/local/bin/creation
ENTRYPOINT ["/usr/local/bin/creation"]
