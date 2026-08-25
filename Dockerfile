# Build stage
FROM golang:1.26.6-alpine3.23 AS builder
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
    go build -ldflags "-w -s -X 'github.com/soulteary/version-kit.Version=$VERSION' -X 'github.com/soulteary/version-kit.Commit=$COMMIT' -X 'github.com/soulteary/version-kit.BuildDate=$BUILD_DATE'" -o herald-dingtalk .

# Runtime stage
FROM alpine:3.23
RUN apk add --no-cache ca-certificates curl && \
    addgroup -S herald && \
    adduser -S -G herald -H -s /sbin/nologin herald
COPY --from=builder /app/herald-dingtalk /bin/herald-dingtalk
EXPOSE 8083
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD curl -fsS http://127.0.0.1:8083/healthz || exit 1
USER herald:herald
CMD ["herald-dingtalk"]
