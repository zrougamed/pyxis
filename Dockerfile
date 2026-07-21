FROM golang:1.26.5-alpine AS builder
WORKDIR /src
RUN apk add --no-cache git
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /out/pyxis ./cmd/

FROM alpine:3.24
RUN apk add --no-cache ca-certificates
COPY --from=builder /out/pyxis /usr/local/bin/pyxis
ENTRYPOINT ["/usr/local/bin/pyxis"]
CMD ["--help"]
