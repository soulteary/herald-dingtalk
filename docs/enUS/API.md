# herald-dingtalk API Documentation

herald-dingtalk implements the HTTP send contract used by Herald's external provider. Request/response types align with [provider-kit](https://github.com/soulteary/provider-kit) `HTTPSendRequest` / `HTTPSendResponse`.

## Base URL

```
http://localhost:8083
```

## Authentication

When `API_KEY` is set, Herald (or any caller) must send the same value in the `X-API-Key` header. If the header is missing or does not match, the server returns `401 Unauthorized` with `error_code: "unauthorized"`.

If `API_KEY` is not set, no authentication is required for `/v1/send`.

## Endpoints

### Health Check

**GET /healthz**

Check service health. Implemented via [health-kit](https://github.com/soulteary/health-kit).

**Response (Success):**
```json
{
  "status": "healthy",
  "service": "herald-dingtalk"
}
```

### Send (DingTalk Work Notification)

**POST /v1/send**

Send a message to a DingTalk user via the work notification API. Called by Herald when channel is `dingtalk`.

**Headers:**
- `X-API-Key` (optional): Required when herald-dingtalk `API_KEY` is set; must match.
- `Idempotency-Key` (optional): Used for idempotent sends; can also be set in the request body as `idempotency_key`.
- `Content-Type`: `application/json`

**Request body (HTTPSendRequest):**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `channel` | string | No | Typically `"dingtalk"` when sent by Herald. |
| `to` | string | Yes | DingTalk user ID (userid). Single user for work notification. |
| `body` | string | No | Message text. If empty, see content resolution below. |
| `idempotency_key` | string | No | Idempotency key; same key within TTL returns cached result. |
| `template` | string | No | Optional; not used for content in current implementation. |
| `params` | object | No | If `body` is empty and `params.code` exists, content becomes `"验证码：" + params.code`. |
| `locale` | string | No | Optional. |
| `subject` | string | No | Optional. |

**Content resolution (in order):**
1. If `body` is non-empty, use `body`.
2. Else if `params.code` exists, use `"验证码：" + params.code`.
3. Else use default: `"您有一条验证消息，请查看。"`

**Response (Success) – HTTP 200:**
```json
{
  "ok": true,
  "message_id": "12345678",
  "provider": "dingtalk"
}
```
`message_id` is the DingTalk async send `task_id` (string).

**Response (Failure):**
```json
{
  "ok": false,
  "error_code": "error_code",
  "error_message": "human-readable message"
}
```

**Error codes and HTTP status:**

| error_code | HTTP status | Description |
|------------|-------------|-------------|
| `unauthorized` | 401 | `API_KEY` is set but `X-API-Key` is missing or invalid. |
| `invalid_request` | 400 | Request body parse error (invalid JSON). |
| `invalid_destination` | 400 | `to` is missing or empty. |
| `provider_down` | 503 | DingTalk not configured (DINGTALK_APP_KEY / DINGTALK_APP_SECRET / DINGTALK_AGENT_ID not set). |
| `send_failed` | 500 | DingTalk API error (e.g. token failure, send failure). |

## Idempotency

- Send requests support idempotency via `Idempotency-Key` header or body field `idempotency_key`.
- Within the configured TTL (`IDEMPOTENCY_TTL_SECONDS`, default 300), a repeated request with the same key returns the cached response (same `ok`, `message_id`, `provider`) without calling DingTalk again.
- Cache is in-memory; key expires after TTL.
