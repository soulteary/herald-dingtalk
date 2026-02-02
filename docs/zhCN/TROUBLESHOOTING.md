# herald-dingtalk 故障排查指南

本文帮助诊断和解决 herald-dingtalk 的常见问题。

## 目录

- [收不到钉钉消息](#收不到钉钉消息)
- [503 provider_down](#503-provider_down)
- [401 Unauthorized](#401-unauthorized)
- [invalid_destination](#invalid_destination)
- [幂等与日志](#幂等与日志)

## 收不到钉钉消息

### 现象

- Herald 使用 channel `dingtalk` 创建 challenge 并收到 herald-dingtalk 成功响应，但用户未收到钉钉消息。

### 排查步骤

1. **查看 herald-dingtalk 日志**  
   关注 `send_failed` 或钉钉 API 报错：
   ```bash
   # Docker 运行时
   docker logs herald-dingtalk 2>&1 | grep -E "send_failed|send ok|errcode"
   ```
   - 若为 `send ok` 且带 `message_id`：herald-dingtalk 已成功调用钉钉；未收到可能是钉钉或用户端问题。
   - 若为 `send_failed` 并带 errmsg：记下钉钉返回的 `errcode`、`errmsg` 继续排查。

2. **核对钉钉配置**  
   - 确认 `DINGTALK_APP_KEY`、`DINGTALK_APP_SECRET`、`DINGTALK_AGENT_ID` 已设置且与开放平台一致。
   - 在钉钉后台确认应用已开通「工作通知」权限且应用已启用/发布。

3. **检查可见范围与 userid**  
   - `to` 必须是钉钉 **userid**（不是手机号或邮箱）。若 Herald 传入错误标识（如手机号），钉钉可能拒绝或无法送达。
   - 确认目标用户在应用可见范围内（全员或指定部门/人员）。

4. **钉钉 API 限流**  
   - 在开放平台查看是否触发频率或配额限制。

### 处理建议

- **凭证错误**：更正 `DINGTALK_APP_KEY`、`DINGTALK_APP_SECRET`、`DINGTALK_AGENT_ID` 并重启 herald-dingtalk。
- **userid 错误或无效**：确保 Herald（或 Warden）将用户解析为正确的钉钉 userid，并以 `destination` 形式传给 channel `dingtalk`。
- **权限或可见范围**：在钉钉后台调整应用权限与可见范围。

---

## 503 provider_down

### 现象

- `POST /v1/send` 返回 HTTP 503，响应体为 `"ok": false, "error_code": "provider_down", "error_message": "dingtalk not configured"`。

### 原因

启动时 herald-dingtalk 会检查三个钉钉配置是否均非空。若 `DINGTALK_APP_KEY`、`DINGTALK_APP_SECRET`、`DINGTALK_AGENT_ID` 任一未设置，则不会初始化钉钉客户端，所有发送请求都会返回 503。

### 处理

1. 补全上述三个环境变量并重启进程（或容器）。
2. 确认运行时确实能读到这些变量（无拼写错误，Docker/K8s 传参正确）。
3. 查看启动日志：若凭证缺失，会打印「未配置钉钉，/v1/send 将返回 503」类警告。

---

## 401 Unauthorized

### 现象

- `POST /v1/send` 返回 HTTP 401，`error_code: "unauthorized"`，`error_message: "invalid or missing API key"`。

### 原因

herald-dingtalk 已配置 `API_KEY`，但请求未携带 `X-API-Key` 或携带的值与配置不一致。

### 处理

1. **若需要 API Key 鉴权**  
   - 在 herald-dingtalk 设置 `API_KEY`。  
   - 在 Herald 设置 `HERALD_DINGTALK_API_KEY` 为相同值，Herald 会将其放在 `X-API-Key` 头中发送。  
   - 确认中间代理/网关未丢弃 `X-API-Key` 头。

2. **若不需要 API Key**  
   - 不要在 herald-dingtalk 设置 `API_KEY`（Herald 侧也不必设置 `HERALD_DINGTALK_API_KEY`）。

---

## invalid_destination

### 现象

- `POST /v1/send` 返回 HTTP 400，`error_code: "invalid_destination"`，`error_message: "to is required"`。

### 原因

请求体中 `to` 为空或未传。对 herald-dingtalk 而言，`to` 必须为钉钉 userid。

### 处理

1. 确保 Herald 在调用 herald-dingtalk 时传入非空的 `to`（即 destination）。对 channel `dingtalk`，Herald 应传入钉钉 userid 作为 destination（可从 Warden 或用户库解析）。
2. 检查「用户标识 → 钉钉 userid」的映射逻辑，避免产生空字符串。

---

## 幂等与日志

### 幂等命中（缓存响应）

当 Herald（或其它客户端）在配置的 TTL 内使用相同的 `Idempotency-Key`（或 body 中的 `idempotency_key`）再次请求时，herald-dingtalk 会直接返回缓存的响应，不再调用钉钉。这是预期行为。

### 日志级别

- **info**：可看到 `send ok`、`send_failed` 以及 503/401 等。
- **debug**：还会看到 `send idempotent hit`，表示命中了幂等缓存。将 `LOG_LEVEL=debug` 可确认重复请求是否被正确缓存。

### TTL

幂等缓存 TTL 由 `IDEMPOTENCY_TTL_SECONDS` 控制（默认 300）。超过 TTL 后，相同 key 会被视为新请求，可能再次调用钉钉发送。
