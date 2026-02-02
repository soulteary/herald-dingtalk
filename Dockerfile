# Build stage
FROM golang:1.25-alpine3.22 AS builder
RUN apk add --no-cache git
WORKDIR /app
ENV CGO_ENABLED=0 GOOS=linux
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -ldflags "-w -s" -o herald-dingtalk .

# Runtime stage
FROM alpine:3.22
RUN apk add --no-cache ca-certificates curl
COPY --from=builder /app/herald-dingtalk /bin/herald-dingtalk
EXPOSE 8083
CMD ["herald-dingtalk"]
