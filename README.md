# herald-dingtalk

DingTalk notification adapter for [Herald](https://github.com/soulteary/herald). Herald sends verification codes via HTTP to this service; herald-dingtalk calls DingTalk work notification API to deliver the message. All DingTalk credentials and business logic live in this project only.

## Protocol

Implements the same HTTP send contract as Herald's external provider (Claude.md 13.1). Request/response types align with [provider-kit](https://github.com/soulteary/provider-kit) `HTTPSendRequest` / `HTTPSendResponse`.

- **POST /v1/send**
  - Request: `channel`, `to` (DingTalk userid), `body` (or `params.code`), `idempotency_key`, optional `template`/`params`/`locale`/`subject`
  - Response: `{ "ok": true, "message_id": "...", "provider": "dingtalk" }` or `{ "ok": false, "error_code": "...", "error_message": "..." }`
- **GET /healthz**: `{ "status": "healthy", "service": "herald-dingtalk" }` (via [health-kit](https://github.com/soulteary/health-kit))

## Configuration

| Env | Description |
|-----|-------------|
| `PORT` | Listen port (default `:8083`) |
| `API_KEY` | Optional; if set, Herald must send `X-API-Key` |
| `DINGTALK_APP_KEY` | DingTalk app key |
| `DINGTALK_APP_SECRET` | DingTalk app secret |
| `DINGTALK_AGENT_ID` | Agent ID for work notification |
| `LOG_LEVEL` | Log level: trace, debug, info, warn, error (default info) |
| `IDEMPOTENCY_TTL_SECONDS` | Idempotency cache TTL (default 300) |

## Herald side

Configure Herald with HTTP provider for channel `dingtalk`:

- `HERALD_DINGTALK_API_URL` = base URL of herald-dingtalk (e.g. `http://herald-dingtalk:8083`)
- Optional: `HERALD_DINGTALK_API_KEY` = same as herald-dingtalk `API_KEY`

Herald does not hold any DingTalk credentials.

## Build & run

```bash
go build -o herald-dingtalk .
./herald-dingtalk
```

With DingTalk credentials in env, `POST /v1/send` will send work notifications to the given userid.

## Testing

```bash
go test ./...
```

Coverage:

```bash
go test -cover ./...
```

Per-function coverage and HTML report:

```bash
go test -coverprofile=coverage.out ./...
go tool cover -func=coverage.out
go tool cover -html=coverage.out
```

Current coverage: `internal/config` (ValidWith), `internal/idempotency` (NewStore/Get/Set). Handler, router, and dingtalk client are not yet covered by unit tests.

## Operation

- **Graceful shutdown**: On `SIGINT` or `SIGTERM`, the server stops accepting new requests and shuts down with a 10s timeout. Logs `"shutting down"` and any shutdown error.
- **Logging**: Structured JSON logs via [logger-kit](https://github.com/soulteary/logger-kit). Key events: send ok (to, message_id), send_failed (err, to), unauthorized, invalid_destination, idempotent hit (debug), 503 provider_down. Set `LOG_LEVEL` to `debug` for idempotent hits.
