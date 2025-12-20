# =========================
# Build stage
# =========================
FROM golang:1.24-alpine AS builder

RUN apk add --no-cache git ca-certificates
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG TARGET=api-server
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/app ./cmd/${TARGET}

# =========================
# Runtime stage
# =========================
FROM alpine:3.20

RUN apk add --no-cache ca-certificates \
 && adduser -D -H -s /sbin/nologin app

USER app
WORKDIR /app

COPY --from=builder /out/app /app/app

# default ports used by our binaries (api:8080, tcp:7070, mirror:9000)
EXPOSE 8080 7070 9000

ENTRYPOINT ["/app/app"]
