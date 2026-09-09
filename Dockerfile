# Build stage
FROM golang:1.27.1-alpine3.24 AS builder
RUN apk add --no-cache git
WORKDIR /app
ENV CGO_ENABLED=0 GOOS=linux
COPY go.mod go.sum ./
RUN go mod download
COPY . .

# Build args for version info (CI/release)
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_DATE
RUN BUILD_DATE=${BUILD_DATE:-$(date +%FT%T%z)} && \
    go build -ldflags "-w -s -X 'github.com/soulteary/version-kit/v2.Version=$VERSION' -X 'github.com/soulteary/version-kit/v2.Commit=$COMMIT' -X 'github.com/soulteary/version-kit/v2.BuildDate=$BUILD_DATE'" -o herald-dingtalk .

# Runtime stage
FROM alpine:3.24
RUN apk add --no-cache ca-certificates curl && \
    addgroup -g 10001 -S herald && \
    adduser -u 10001 -S -D -G herald -H -s /sbin/nologin herald
COPY --from=builder /app/herald-dingtalk /bin/herald-dingtalk
EXPOSE 8083
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD port="${PORT:-8083}"; port="${port#:}"; \
        curl -fsS "http://127.0.0.1:${port}/healthz" || exit 1
USER 10001:10001
CMD ["herald-dingtalk"]
