# syntax=docker/dockerfile:1
FROM golang:1.26.8-alpine AS build
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
ENV TMPDIR=/work
USER 65532:65532
EXPOSE 8080 8081 8082
ENTRYPOINT ["/usr/local/bin/lanverse"]
CMD ["--role=api"]
