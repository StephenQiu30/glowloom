FROM golang:1.26.11-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/media ./cmd/media

FROM debian:12-slim
RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates curl ffmpeg \
    && rm -rf /var/lib/apt/lists/* \
    && mkdir /work && chown 65532:65532 /work && chmod 700 /work
ENV TMPDIR=/work
USER 65532:65532
EXPOSE 9091 8081
ENTRYPOINT ["/usr/local/bin/media"]
COPY --from=build /out/media /usr/local/bin/media
