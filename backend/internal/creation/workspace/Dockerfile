FROM golang:1.26.11-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/creation-workspace ./creation/cmd/workspace

FROM alpine:3.23
RUN apk add --no-cache ca-certificates \
    && mkdir /work && chown 65532:65532 /work && chmod 700 /work
ENV TMPDIR=/work
USER 65532:65532
COPY --from=build /out/creation-workspace /usr/local/bin/creation-workspace
ENTRYPOINT ["/usr/local/bin/creation-workspace"]
