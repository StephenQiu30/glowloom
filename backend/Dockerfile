# syntax=docker/dockerfile:1
FROM golang:1.26.8-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/lanverse ./cmd/lanverse

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/lanverse /usr/local/bin/lanverse
USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/lanverse"]
CMD ["--role=api"]
