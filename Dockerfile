# Build stage
FROM golang:1.25-alpine3.22 AS builder
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
FROM alpine:3.22
RUN apk add --no-cache ca-certificates curl
COPY --from=builder /app/herald-dingtalk /bin/herald-dingtalk
EXPOSE 8083
CMD ["herald-dingtalk"]
