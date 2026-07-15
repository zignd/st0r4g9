# syntax=docker/dockerfile:1

# --- build stage -----------------------------------------------------------
# modernc.org/sqlite is pure Go, so we build a fully static binary (CGO off)
# and can ship it on a minimal base.
FROM golang:1.26-alpine AS build

WORKDIR /src
ENV CGO_ENABLED=0 GOTOOLCHAIN=local

# Cache module downloads separately from the source.
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go build -trimpath -ldflags="-s -w" -o /out/st0r4g9 ./cmd/st0r4g9

# --- runtime stage ---------------------------------------------------------
FROM alpine:3.20

RUN apk add --no-cache ca-certificates

COPY --from=build /out/st0r4g9 /usr/local/bin/st0r4g9

# Object bytes (blobs) and the SQLite DB live under /data, which is bind-mounted
# from the host so data survives the container being dropped.
ENV ST0R4G9_ADDR=":9000" \
    ST0R4G9_DB="/data/store.db" \
    ST0R4G9_DATA_DIR="/data/blobs"

EXPOSE 9000
ENTRYPOINT ["st0r4g9"]
