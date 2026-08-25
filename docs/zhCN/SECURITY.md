# herald-dingtalk 安全实践

本文说明 herald-dingtalk 的安全注意事项与推荐做法。

## API Key

- 配置 `API_KEY` 后，herald-dingtalk 会要求请求头 `X-API-Key` 与之一致；适用于 **POST /v1/send** 与 **POST /v1/resolve**。请使用足够强且唯一的密钥并妥善保管。
- Herald 侧需配置相同的 `HERALD_DINGTALK_API_KEY`，以便在请求 herald-dingtalk 时携带该密钥；Stargate 等调用 `/v1/resolve` 时也需携带相同密钥（若已配置）。
- 不要将 API Key 写入日志或对外暴露。优先使用环境变量或密钥管理服务，避免将密钥写入并提交到仓库的配置文件中。
- API Key 校验会先对双方取哈希，再进行常量时间比较，减少简单的时序侧信道。

## 钉钉凭证

- **AppKey**、**AppSecret**、**AgentID** 不得硬编码或提交到代码库。
- 应通过环境变量或密钥管理服务（如 Kubernetes Secrets、HashiCorp Vault）注入。本地开发可使用 `.env`，并确保 `.env` 已加入 `.gitignore`。
- 建议在钉钉开放平台定期轮换 AppSecret，并同步更新 herald-dingtalk 的配置。

## 身份与授权边界

- `/v1/resolve` 将钉钉 OAuth2 授权码兑换为钉钉 `userid`。该结果是消息投递标识，不代表用户有权访问某个应用或资源。
- herald-dingtalk 不创建应用会话、不校验 OTP challenge、不把企业身份映射为应用账号，也不做业务授权判断。这些职责应保留在 Herald、Warden、Stargate 或应用自身的用户系统中。
- `API_KEY` 用于认证调用本适配器的服务。它是共享的服务凭证，不是终端用户凭证，不能代替逐用户授权。
- `/v1/resolve` 与 `/v1/send` 应采用同等严格的访问控制：前者接收短时有效的 OAuth 材料，并返回企业身份标识。

## 生产环境建议

- **网络**：将 herald-dingtalk 部署在内网或私有网络中，仅允许 Herald（或统一网关）访问；不要将 herald-dingtalk 直接暴露到公网，除非在 HTTPS 与严格访问控制之后。
- **HTTPS**：若 herald-dingtalk 会经过公网或不可信网络被访问，应在其前增加带 TLS 的反向代理（如 Traefik、nginx）。此时 Herald 的 `HERALD_DINGTALK_API_URL` 应使用 `https://`。
- **最小权限**：使用非 root 用户运行进程；在 Docker 中尽量使用非 root 用户镜像。
- **请求边界**：请求体默认限制为 64 KiB（最高可配置到 1 MiB），HTTP 读写设置超时，并通过 panic 恢复避免单个处理器异常终止进程。
- **日志**：结构化访问日志仅记录 method、path、status、latency、客户端元数据和请求 ID，不记录请求头、查询字符串、请求体、接收者标识、手机号、userid 或 OAuth 授权码；业务事件使用相同请求 ID 便于关联。
- **运维**：密钥注入、探针、上线验证和多副本幂等限制见 [运维指南](OPERATIONS.md)。

## 小结

- 生产环境建议配置 `API_KEY` 并严格保密；Herald 侧配置 `HERALD_DINGTALK_API_KEY` 与之一致。
- 钉钉凭证仅通过环境变量或密钥管理服务注入，不写入代码或提交的配置。
- 尽量在内网部署 herald-dingtalk，对外暴露时使用 HTTPS 与访问控制。
