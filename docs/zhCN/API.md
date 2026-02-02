# herald-dingtalk API 文档

herald-dingtalk 实现 Herald 外部 Provider 使用的 HTTP 发送协议，请求/响应类型与 [provider-kit](https://github.com/soulteary/provider-kit) 的 `HTTPSendRequest` / `HTTPSendResponse` 一致。

## Base URL

```
http://localhost:8083
```

## 认证

当配置了 `API_KEY` 时，Herald（或任意调用方）必须在请求头中携带 `X-API-Key`，且值与 herald-dingtalk 的 `API_KEY` 一致。若未携带或不一致，返回 `401 Unauthorized`，`error_code` 为 `"unauthorized"`。

未配置 `API_KEY` 时，`/v1/send` 不需要认证。

## 端点

### 健康检查

**GET /healthz**

检查服务健康状态，由 [health-kit](https://github.com/soulteary/health-kit) 实现。

**成功响应：**
```json
{
  "status": "healthy",
  "service": "herald-dingtalk"
}
```

### 发送（钉钉工作通知）

**POST /v1/send**

通过钉钉工作通知 API 向指定用户发送消息。由 Herald 在 channel 为 `dingtalk` 时调用。

**请求头：**
- `X-API-Key`（可选）：当 herald-dingtalk 配置了 `API_KEY` 时必传且需一致。
- `Idempotency-Key`（可选）：用于幂等发送；也可在请求体中通过 `idempotency_key` 传递。
- `Content-Type`：`application/json`

**请求体（HTTPSendRequest）：**

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `channel` | string | 否 | Herald 调用时通常为 `"dingtalk"`。 |
| `to` | string | 是 | 钉钉用户 ID（userid），工作通知为单用户。 |
| `body` | string | 否 | 消息正文。为空时见下方内容解析规则。 |
| `idempotency_key` | string | 否 | 幂等键；TTL 内相同 key 返回缓存结果。 |
| `template` | string | 否 | 可选；当前实现未用于内容。 |
| `params` | object | 否 | 当 `body` 为空且存在 `params.code` 时，内容为「验证码：」+ params.code。 |
| `locale` | string | 否 | 可选。 |
| `subject` | string | 否 | 可选。 |

**内容解析顺序：**
1. 若 `body` 非空，使用 `body`。
2. 否则若存在 `params.code`，使用「验证码：」+ `params.code`。
3. 否则使用默认文案：「您有一条验证消息，请查看。」

**成功响应 – HTTP 200：**
```json
{
  "ok": true,
  "message_id": "12345678",
  "provider": "dingtalk"
}
```
`message_id` 为钉钉异步发送返回的 `task_id`（字符串）。

**失败响应：**
```json
{
  "ok": false,
  "error_code": "错误码",
  "error_message": "可读说明"
}
```

**错误码与 HTTP 状态：**

| error_code | HTTP 状态 | 说明 |
|------------|-----------|------|
| `unauthorized` | 401 | 已配置 `API_KEY` 但未传或错误的 `X-API-Key`。 |
| `invalid_request` | 400 | 请求体解析失败（如非法 JSON）。 |
| `invalid_destination` | 400 | `to` 为空或未传。 |
| `provider_down` | 503 | 未配置钉钉（未设置 DINGTALK_APP_KEY / DINGTALK_APP_SECRET / DINGTALK_AGENT_ID）。 |
| `send_failed` | 500 | 钉钉 API 调用失败（如 token 失败、发送失败）。 |

## 幂等

- 发送请求支持通过请求头 `Idempotency-Key` 或 body 字段 `idempotency_key` 做幂等。
- 在配置的 TTL 内（`IDEMPOTENCY_TTL_SECONDS`，默认 300 秒），相同 key 的重复请求会直接返回缓存的响应（相同的 `ok`、`message_id`、`provider`），不再调用钉钉 API。
- 缓存在进程内存中，超过 TTL 后 key 失效。
