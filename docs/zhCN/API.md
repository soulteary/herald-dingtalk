# herald-dingtalk API 文档

herald-dingtalk 实现 Herald 外部 Provider 使用的 HTTP 发送协议，请求/响应类型与 [provider-kit](https://github.com/soulteary/provider-kit) 的 `HTTPSendRequest` / `HTTPSendResponse` 一致。

## Base URL

```
http://localhost:8083
```

## 认证

当配置了 `API_KEY` 时，Herald（或任意调用方）必须在请求头中携带 `X-API-Key`，且值与 herald-dingtalk 的 `API_KEY` 一致。若未携带或不一致，返回 `401 Unauthorized`，`error_code` 为 `"unauthorized"`。

未配置 `API_KEY` 时，`/v1/send` 与 `/v1/resolve` 均不需要认证。

API Key 使用固定长度的常量时间比较。所有响应均包含 `X-Request-ID`；调用方提供该请求头时会沿用其值。

## 通用 HTTP 行为

- 超过 `MAX_REQUEST_BODY_BYTES`（默认 64 KiB）的请求会返回 HTTP `413 Request Entity Too Large`。该响应由 HTTP 服务器生成，不使用 Provider 错误结构。
- 每个进程最多同时处理 `MAX_CONCURRENT_REQUESTS` 个 `/v1` 请求（默认 32）。超限请求返回 HTTP `429`、`error_code: "rate_limited"` 和 `Retry-After: 1`。设置为 `0` 可禁用此限制。
- JSON 端点要求有效的 `Content-Type: application/json` 媒体类型；允许 `charset=utf-8` 等参数。
- 使用响应中的 `X-Request-ID` 关联该请求的访问日志和业务事件。请求体、凭证与接收者标识不会写入日志。

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

**GET /readyz**

检查钉钉配置是否具有正确语义。仅当凭证完整且无首尾空白、`DINGTALK_AGENT_ID` 为十进制正整数，并且 `DINGTALK_LOOKUP_MODE` 为 `none` 或 `mobile` 时，才返回 `200` 和 `status: "ready"`；否则返回 `503` 和 `status: "not_ready"`。`/healthz` 用于存活检查，`/readyz` 用于就绪检查。

### 解析 OAuth2 授权码（可选）

**POST /v1/resolve**

将钉钉 OAuth2 授权链接回调得到的 `auth_code` 兑换为钉钉 **userid**。适用于 Stargate 使用钉钉 OAuth2 登录流程时，在服务端用 code 换取 userid 再创建 session。

依据钉钉开放平台 [OAuth2 鉴权](https://open.dingtalk.com/document/connection/oauth2-0-authentication) 与 [获取登录用户访问凭证](https://open.dingtalk.com/document/orgapp/obtain-identity-credentials)：`/v1.0/oauth2/userAccessToken` 换 token，再调用 `/v1.0/contact/users/me` 取 userid。

**请求头：**
- `X-API-Key`（可选）：当 herald-dingtalk 配置了 `API_KEY` 时必传且需一致。
- `Content-Type`：`application/json`

**请求体：**

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `auth_code` | string | 是 | 钉钉 OAuth2 授权后回调参数中的 code；1–4096 字节，且不能包含首尾空白或控制字符。 |

**成功响应 – HTTP 200：**
```json
{
  "ok": true,
  "userid": "xxx"
}
```

**失败响应：**
```json
{
  "ok": false,
  "error_code": "resolve_failed",
  "error_message": "可读说明"
}
```

| error_code | HTTP 状态 | 说明 |
|------------|-----------|------|
| `unauthorized` | 401 | 已配置 `API_KEY` 但未传或错误的 `X-API-Key`。 |
| `rate_limited` | 429 | 当前进程已达到 `MAX_CONCURRENT_REQUESTS`，或钉钉对 OAuth2 兑换实施限流。 |
| `unsupported_media_type` | 415 | `Content-Type` 不是 `application/json`。 |
| `invalid_request` | 400 | 请求体解析失败，或 `auth_code` 为空、过长、包含首尾空白/控制字符。 |
| `provider_down` | 503 | 钉钉配置未通过本地校验，或钉钉拒绝当前服务凭证。 |
| `resolve_failed` | 400 / 502 | 授权码无效或过期时返回 400；钉钉 OAuth2 服务异常时返回 502。 |
| `timeout` | 504 | 钉钉 OAuth2 兑换超时。 |

---

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
| `channel` | string | 否 | 传入时必须为 `"dingtalk"`；为兼容旧调用，空值仍然接受。 |
| `to` | string | 是 | 钉钉 **userid**，或（当 `DINGTALK_LOOKUP_MODE=mobile` 时）11 位**手机号**；1–256 字节，且不能包含首尾空白或控制字符。 |
| `body` | string | 否 | 消息正文。为空时见下方内容解析规则。 |
| `idempotency_key` | string | 否 | 幂等键，最长 256 字节，且不能包含首尾空白或控制字符；TTL 内缓存成功结果。 |
| `template` | string | 否 | 可选；当前实现未用于内容。 |
| `params` | object | 否 | 当 `body` 为空且存在 `params.code` 时，内容为「验证码：」+ params.code。 |
| `locale` | string | 否 | 可选。 |
| `subject` | string | 否 | 可选。 |
| `timeout_seconds` | integer | 否 | 钉钉操作的端到端超时秒数，范围为 0–30；0 表示使用服务端默认值 25 秒。 |

**destination（to）支持：**
- **`DINGTALK_LOOKUP_MODE=none`**（默认）：`to` 仅支持钉钉 **userid**。
- **`DINGTALK_LOOKUP_MODE=mobile`**：`to` 支持 **userid** 或 **11 位手机号**；为手机号时会调用钉钉「根据手机号查询用户」接口解析为 userid 再发送。需在钉钉开放平台为应用申请 **Contact.User.mobile**（根据手机号查询用户）权限。

**内容解析顺序：**
1. 若 `body` 非空，使用 `body`。
2. 否则若存在 `params.code`，使用「验证码：」+ `params.code`。
3. 否则使用默认文案：「您有一条验证消息，请查看。」

**关于模板消息：** 钉钉官方说明：**模板消息（sendbytemplate）不支持企业内部应用。** 本服务使用企业内部应用 + 工作通知（文本消息），不适用也不使用消息模板。

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
| `rate_limited` | 429 | 当前进程已达到 `MAX_CONCURRENT_REQUESTS`，或钉钉对发送操作实施限流。 |
| `unsupported_media_type` | 415 | `Content-Type` 不是 `application/json`。 |
| `invalid_request` | 400 | 非法 JSON、不支持的 channel、幂等键过长，或 `timeout_seconds` 超出 0–30。 |
| `invalid_destination` | 400 | `to` 未通过本地校验，或钉钉报告目标用户不可用。 |
| `idempotency_conflict` | 409 | Header/body 中的 key 不一致，或同一 key 被用于不同发送内容。 |
| `provider_down` | 503 | 钉钉配置未通过本地校验，或钉钉拒绝当前服务凭证。 |
| `send_failed` | 502 | 钉钉 API 调用失败（如 token 失败、发送失败）。 |
| `timeout` | 504 | 请求级超时到期，或钉钉请求被取消。 |

## 幂等

- 发送请求支持通过请求头 `Idempotency-Key` 或 body 字段 `idempotency_key` 做幂等；两者同时存在时必须一致。
- 相同 key、相同发送内容的并发请求只执行一次钉钉调用，等待方共享相同结果。
- 成功结果在 `IDEMPOTENCY_TTL_SECONDS`（默认 300 秒）内缓存。失败结果只共享给当前等待方，不进入缓存，后续请求可以重试。
- key 与 channel、目标、正文、模板、参数、主题和 locale 绑定；使用同一 key 发送不同内容时返回 HTTP 409。
- 缓存位于进程内，最多保留 10000 条成功记录，并在后续操作中清理过期项。多副本部署如需跨实例幂等，应使用共享存储。
