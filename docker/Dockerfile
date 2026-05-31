# syntax=docker/dockerfile:1
# Build context is the project root (see docker/docker-compose.yml).

# --- build stage: compile a static, CGO-free binary ---
FROM golang:1.26-alpine AS build
ENV CGO_ENABLED=0 GOTOOLCHAIN=local
WORKDIR /src

# Cache module downloads separately from the source for faster rebuilds.
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go build -ldflags="-s -w" -o /out/foxlogi .

# --- runtime stage: minimal image with CA certs for Discord's TLS ---
FROM alpine:3.21
RUN apk add --no-cache ca-certificates \
    && adduser -D -u 10001 foxlogi \
    && mkdir -p /data \
    && chown foxlogi:foxlogi /data
USER foxlogi
WORKDIR /app
COPY --from=build /out/foxlogi /app/foxlogi

# Persist the SQLite database on a volume by default.
ENV CRAFT_DB_PATH=/data/foxlogi.db
VOLUME ["/data"]

ENTRYPOINT ["/app/foxlogi"]
