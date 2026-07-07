# ── Stage 1: frontend ────────────────────────────────────────────────────────
FROM node:20-alpine AS web
WORKDIR /build/web
COPY web/package.json web/package-lock.json* ./
RUN npm install
COPY web/ ./
COPY static/ /build/static/
RUN npm run build

# ── Stage 2: backend ─────────────────────────────────────────────────────────
FROM golang:1.26-alpine AS backend
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ cmd/
COPY internal/ internal/
# modernc.org/sqlite is pure Go — no CGO needed
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o yata ./cmd/yata

# ── Stage 3: runtime ─────────────────────────────────────────────────────────
FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app

COPY --from=backend /build/yata /app/yata
COPY --from=web /build/static /app/static
COPY templates/ /app/templates/
COPY defs/ /app/defs/
COPY test_data.json /app/test_data.json

# /data holds config.json + the SQLite database (mount a volume here).
# defs/ and static/themes/ can also be mounted to customise without rebuilds.
ENV YATA_CONFIG=/data/config.json \
    YATA_DATA=/data/yata.db \
    YATA_DEFS=/app/defs \
    YATA_BASE=/app \
    YATA_PORT=8420
VOLUME ["/data"]
EXPOSE 8420

ENTRYPOINT ["/app/yata"]
