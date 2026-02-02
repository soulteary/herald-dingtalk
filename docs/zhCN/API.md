# herald-dingtalk API 文档

herald-dingtalk 实现 Herald 外部 Provider 使用的 HTTP 发送协议，请求/响应类型与 [provider-kit](https://github.com/soulteary/provider-kit) 的 `HTTPSendRequest` / `HTTPSendResponse` 一致。

## Base URL

```
http://localhost:8083
```

## 认证

当配置了 `API_KEY` 时，Herald（或任意调用方）必须在请求头中携带 `X-API-Key`，且值与 herald-dingtalk 的 `API_KEY` 一致。若未携带或不一致，返回 `401 Unauthorized`，`error_code` 为 `"unauthorized"`。

未配置 `API_KEY` 时，`/v1/send` 与 `/v1/resolve` 均不需要认证。

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
| `auth_code` | string | 是 | 钉钉 OAuth2 授权后回调参数中的 code。 |

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
| `invalid_request` | 400 | 请求体解析失败或 `auth_code` 为空。 |
| `provider_down` | 503 | 未配置钉钉凭证。 |
| `resolve_failed` | 400 | OAuth2 兑换失败（code 过期、无效等）。 |

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
| `channel` | string | 否 | Herald 调用时通常为 `"dingtalk"`。 |
| `to` | string | 是 | 钉钉 **userid**，或（当 `DINGTALK_LOOKUP_MODE=mobile` 时）11 位**手机号**；工作通知为单用户。 |
| `body` | string | 否 | 消息正文。为空时见下方内容解析规则。 |
| `idempotency_key` | string | 否 | 幂等键；TTL 内相同 key 返回缓存结果。 |
| `template` | string | 否 | 可选；当前实现未用于内容。 |
| `params` | object | 否 | 当 `body` 为空且存在 `params.code` 时，内容为「验证码：」+ params.code。 |
| `locale` | string | 否 | 可选。 |
| `subject` | string | 否 | 可选。 |

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
| `invalid_request` | 400 | 请求体解析失败（如非法 JSON）。 |
| `invalid_destination` | 400 | `to` 为空或未传。 |
| `provider_down` | 503 | 未配置钉钉（未设置 DINGTALK_APP_KEY / DINGTALK_APP_SECRET / DINGTALK_AGENT_ID）。 |
| `send_failed` | 500 | 钉钉 API 调用失败（如 token 失败、发送失败）。 |

## 幂等

- 发送请求支持通过请求头 `Idempotency-Key` 或 body 字段 `idempotency_key` 做幂等。
- 在配置的 TTL 内（`IDEMPOTENCY_TTL_SECONDS`，默认 300 秒），相同 key 的重复请求会直接返回缓存的响应（相同的 `ok`、`message_id`、`provider`），不再调用钉钉 API。
- 缓存在进程内存中，超过 TTL 后 key 失效。
