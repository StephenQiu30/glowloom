FROM golang:1.26.11 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -o /out/identity ./identity

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/identity /identity
USER 65532:65532
ENTRYPOINT ["/identity"]
